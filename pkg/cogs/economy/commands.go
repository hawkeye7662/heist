package economy

import (
	"strings"
	"time"

	"github.com/bwmarrin/discordgo"
	"github.com/olekukonko/tablewriter"
	"github.com/rbrabson/heist/pkg/format"
	"github.com/rbrabson/heist/pkg/msg"
	log "github.com/sirupsen/logrus"
)

var (
	componentHandlers = map[string]func(s *discordgo.Session, i *discordgo.InteractionCreate){
		"montly_leaderboard":   monthlyLeaderboard,
		"current_leaderboard":  currentLeaderboard,
		"lifetime_leaderboard": lifetimeLeaderboard,
		"monthly_rank":         monthlyRank,
		"current_rank":         currentRank,
		"lifetime_rank":        lifetimeRank,
	}

	commandHandlers = map[string]func(s *discordgo.Session, i *discordgo.InteractionCreate){
		"account":     bankAccount,
		"balance":     getAccountInfo,
		"bank":        bank,
		"leaderboard": leaderboard,
		"transfer":    transferCredits,
	}

	adminCommands = []*discordgo.ApplicationCommand{
		{
			Name:        "bank",
			Description: "Commands used to interact with the economy for this server.",
			Options: []*discordgo.ApplicationCommandOption{
				{
					Name:        "account",
					Description: "Gets the bank account information for the given member.",
					Type:        discordgo.ApplicationCommandOptionSubCommand,
					Options: []*discordgo.ApplicationCommandOption{
						{
							Type:        discordgo.ApplicationCommandOptionString,
							Name:        "id",
							Description: "The member ID.",
							Required:    true,
						},
					},
				},
				{
					Name:        "set",
					Description: "Sets the amount of credits for a given member.",
					Type:        discordgo.ApplicationCommandOptionSubCommand,
					Options: []*discordgo.ApplicationCommandOption{
						{
							Type:        discordgo.ApplicationCommandOptionString,
							Name:        "id",
							Description: "The member ID.",
							Required:    true,
						},
						{
							Type:        discordgo.ApplicationCommandOptionInteger,
							Name:        "amount",
							Description: "The amount to set the account to.",
							Required:    true,
						},
					},
				},
				{
					Name:        "transfer",
					Description: "Transfers the account balance from one account to another.",
					Type:        discordgo.ApplicationCommandOptionSubCommand,
					Options: []*discordgo.ApplicationCommandOption{
						{
							Type:        discordgo.ApplicationCommandOptionString,
							Name:        "from",
							Description: "The ID of the account to transfer credits from.",
							Required:    true,
						},
						{
							Type:        discordgo.ApplicationCommandOptionInteger,
							Name:        "to",
							Description: "The ID of the account to receive account balance.",
							Required:    true,
						},
					},
				},
			},
		},
	}

	memberCommands = []*discordgo.ApplicationCommand{
		{
			Name:        "leaderboard",
			Description: "Gets the global economy leaderboard.",
		},
		{
			Name:        "transfer",
			Description: "Transfers a set amount of credits from your account to another player's account.",
			Options: []*discordgo.ApplicationCommandOption{
				{
					Type:        discordgo.ApplicationCommandOptionString,
					Name:        "to",
					Description: "The ID of the member to transfer credits to.",
					Required:    true,
				},
				{
					Type:        discordgo.ApplicationCommandOptionInteger,
					Name:        "amount",
					Description: "The amount of credits to transfer.",
					Required:    true,
				},
			},
		},
		{
			Name:        "balance",
			Description: "Bank account balance for the member",
		},
	}
)

// bank routes the bank commands to the proper handers.
func bank(s *discordgo.Session, i *discordgo.InteractionCreate) {
	log.Trace("--> bank")
	defer log.Trace("<-- bank")

	options := i.ApplicationCommandData().Options
	switch options[0].Name {
	case "set":
		setAccount(s, i)
	case "transfer":
		transferAccount(s, i)
	case "account":
		bankAccount(s, i)
	}
}

// transferCredits removes a specified amount of credits from initiators account and deposits them in the target's account.
func transferCredits(s *discordgo.Session, i *discordgo.InteractionCreate) {
	log.Trace("--> bank")
	defer log.Trace("<-- bank")

	var toID string
	var amount int
	options := i.ApplicationCommandData().Options
	for _, option := range options {
		switch option.Name {
		case "to":
			toID = option.StringValue()
		case "amount":
			amount = int(option.IntValue())
		}
	}

	p := getPrinter(i)

	member, err := s.GuildMember(i.GuildID, toID)
	if err != nil {
		resp := p.Sprintf("A account with ID `%s` is not a member of this server", toID)
		msg.SendEphemeralResponse(s, i, resp)
		return
	}

	bank := GetBank(i.GuildID)
	fromAccount := bank.GetAccount(i.Member.User.ID, getMemberName(i.Member.User.ID, i.Member.Nick))
	toAccount := bank.GetAccount(toID, getMemberName(member.User.ID, member.Nick))

	if fromAccount.NextTransferOut.After(time.Now()) {
		duration := time.Until(fromAccount.NextTransferOut)
		resp := p.Sprintf("You can't transfer credits yet. Please wait %s to transfer credits.", format.Duration(duration))
		msg.SendEphemeralResponse(s, i, resp)
		return
	}
	if toAccount.NextTransferIn.After(time.Now()) {
		duration := time.Until(toAccount.NextTransferIn)
		resp := p.Sprintf("%s can't receive credits yet. Please wait %s to transfer credits.", format.Duration(duration))
		msg.SendEphemeralResponse(s, i, resp)
		return
	}
	if amount > bank.MaxTransferAmount {
		resp := p.Sprintf("You can only transfer a maximum of %d credits.", bank.MaxTransferAmount)
		msg.SendEphemeralResponse(s, i, resp)
		return
	}
	if fromAccount.CurrentBalance < amount {
		resp := p.Sprintf("Your can't transfer %d credits as your account only has %d credits", amount, fromAccount.CurrentBalance)
		msg.SendEphemeralResponse(s, i, resp)
		return
	}

	log.WithFields(log.Fields{
		"From":         fromAccount.Name,
		"To":           toAccount.Name,
		"Amount":       amount,
		"From Balance": fromAccount.CurrentBalance,
		"To Balance":   toAccount.CurrentBalance,
	}).Debug("/transfer")

	WithdrawCredits(bank, fromAccount, amount)
	DepositCredits(bank, toAccount, amount)
	fromAccount.NextTransferOut = time.Now().Add(bank.MinTransferDuration)
	toAccount.NextTransferIn = time.Now().Add(bank.MinTransferDuration)
	SaveBank(bank)
	resp := p.Sprintf("You transfered %d credits to %s's account.", amount, toAccount.Name)
	msg.SendResponse(s, i, resp)
}

// bankAccount returns information about a bank account for the specified member.
func bankAccount(s *discordgo.Session, i *discordgo.InteractionCreate) {
	log.Trace("--> bankAccount")
	defer log.Trace("<-- bankAccount")

	p := getPrinter(i)

	bank := GetBank(i.GuildID)
	accountID := i.ApplicationCommandData().Options[0].Options[0].StringValue()
	account, ok := bank.Accounts[accountID]
	if !ok {
		resp := p.Sprintf("The bank account for member %s could not be found.", accountID)
		msg.SendEphemeralResponse(s, i, resp)
		return
	}

	resp := p.Sprintf("**ID**: %s\n**Name**: %s\n**Balance**: %d\n**GlobalRanking**: %d\n**Created**: %s\n**NextTransferIn**: %s\n**NextTransferOut**: %s", account.ID, account.Name, account.CurrentBalance, GetMonthlyRanking(bank.ID, account.ID), account.CreatedAt, account.NextTransferIn, account.NextTransferOut)
	msg.SendEphemeralResponse(s, i, resp)
}

// getAccountInfo returns information about a member's bank account to that member.
func getAccountInfo(s *discordgo.Session, i *discordgo.InteractionCreate) {
	log.Trace("--> accountInfo")
	defer log.Trace("<-- accountInfo")

	p := getPrinter(i)

	bank := GetBank(i.GuildID)
	account := bank.GetAccount(i.Member.User.ID, getMemberName(i.Member.User.Username, i.Member.Nick))
	resp := p.Sprintf("**Name**: %s\n**Current Balance**: %d, **Ranking**: %d\n**Lifetime Balance**: %d, **Ranking** %d", account.Name, account.CurrentBalance, GetMonthlyRanking(bank.ID, account.ID), account.LifetimeBalance, GetLifetimeRanking(bank.ID, account.ID))
	msg.SendEphemeralResponse(s, i, resp)
}

// setAccount sets the account to the specified number of credits.
func setAccount(s *discordgo.Session, i *discordgo.InteractionCreate) {
	log.Trace("--> setAccount")
	defer log.Trace("<-- setAccount")

	var id string
	var amount int
	options := i.ApplicationCommandData().Options[0].Options
	for _, option := range options {
		switch option.Name {
		case "id":
			id = strings.TrimSpace(option.StringValue())
		case "amount":
			amount = int(option.IntValue())
		}
	}

	p := getPrinter(i)

	member, err := s.GuildMember(i.GuildID, id)
	if err != nil {
		resp := p.Sprintf("A account with ID `%s` is not a member of this server", id)
		msg.SendEphemeralResponse(s, i, resp)
		return
	}

	bank := GetBank(i.GuildID)
	account := bank.GetAccount(id, getMemberName(member.User.ID, member.Nick))
	account.MonthlyBalance = amount
	account.CurrentBalance = amount
	account.LifetimeBalance = amount

	log.WithFields(log.Fields{
		"Account": account.Name,
		"Amount":  amount,
	}).Debug("/bank set")

	SaveBank(bank)

	resp := p.Sprintf("Account for %s was set to %d credits.", account.Name, account.CurrentBalance)
	msg.SendResponse(s, i, resp)
}

// transferAccount sets the target account to the amount of credits in the source
// account, and clears the account balance of the source.
func transferAccount(s *discordgo.Session, i *discordgo.InteractionCreate) {
	log.Trace("--> transferAccount")
	defer log.Trace("<-- transferAccount")

	var fromID, toID string
	options := i.ApplicationCommandData().Options[0].Options
	for _, option := range options {
		switch option.Name {
		case "from":
			fromID = strings.TrimSpace(option.StringValue())
		case "to":
			toID = strings.TrimSpace(option.StringValue())
		}
	}

	p := getPrinter(i)

	bank := GetBank(i.GuildID)
	fromAccount, ok := bank.Accounts[fromID]
	if !ok {
		resp := p.Sprintf("Account %s does not exist.")
		msg.SendEphemeralResponse(s, i, resp)
		return
	}

	member, err := s.GuildMember(i.GuildID, toID)
	if err != nil {
		resp := p.Sprintf("An account with ID `%s` is not a member of this server", toID)
		msg.SendEphemeralResponse(s, i, resp)
		return
	}

	toAccount := bank.GetAccount(toID, getMemberName(member.User.Username, member.Nick))

	toAccount.MonthlyBalance = fromAccount.MonthlyBalance
	toAccount.CurrentBalance = fromAccount.CurrentBalance
	toAccount.LifetimeBalance = fromAccount.LifetimeBalance
	fromAccount.MonthlyBalance = 0
	fromAccount.CurrentBalance = 0
	fromAccount.LifetimeBalance = 0

	log.WithFields(log.Fields{
		"From":    fromAccount.Name,
		"To":      toAccount.Name,
		"Balance": toAccount.CurrentBalance,
	}).Debug("/bank transfer")

	SaveBank(bank)

	resp := p.Sprintf("Transferred balance of %d from %s to %s.", toAccount.CurrentBalance, fromAccount.Name, toAccount.Name)
	msg.SendResponse(s, i, resp)

}

// leaderboard returns the top 10 players in the server's economy.
func leaderboard(s *discordgo.Session, i *discordgo.InteractionCreate) {
	log.Trace("--> leaderboard")
	defer log.Trace("<-- leaderboard")

	p := getPrinter(i)

	accounts := GetMonthlyLeaderboard(i.GuildID, 10)

	var tableBuffer strings.Builder
	table := tablewriter.NewWriter(&tableBuffer)
	table.SetColumnSeparator(" ")
	table.SetCenterSeparator(" ")
	table.SetBorder(false)
	table.SetHeader([]string{"Name", "Balance"})
	for _, account := range accounts {
		data := []string{account.Name, p.Sprintf("%d", account.CurrentBalance)}
		table.Append(data)
	}
	table.Render()
	msg.SendEphemeralResponse(s, i, "```\n"+tableBuffer.String()+"\n```")
}

// monthlyLeaderboard returns the top 10 players in the server's economy for the current month.
func monthlyLeaderboard(s *discordgo.Session, i *discordgo.InteractionCreate) {
	log.Trace("--> leaderboard")
	defer log.Trace("<-- leaderboard")
}

// currentLeaderboard returns the top 10 players in the server's economy based on their current account balance.
func currentLeaderboard(s *discordgo.Session, i *discordgo.InteractionCreate) {
	log.Trace("--> leaderboard")
	defer log.Trace("<-- leaderboard")
}

// lifetimeLeaderboard returns the top 10 players in the server's economy based on the amount of credits
// deposited over the lifetime of the account.
func lifetimeLeaderboard(s *discordgo.Session, i *discordgo.InteractionCreate) {
	log.Trace("--> leaderboard")
	defer log.Trace("<-- leaderboard")
}

// monthlyRank returns the ranking of the player for the current month.
func monthlyRank(s *discordgo.Session, i *discordgo.InteractionCreate) {
	log.Trace("--> leaderboard")
	defer log.Trace("<-- leaderboard")
}

// monthlyRank returns the ranking of the player for the current balance.
func currentRank(s *discordgo.Session, i *discordgo.InteractionCreate) {
	log.Trace("--> leaderboard")
	defer log.Trace("<-- leaderboard")
}

// monthlyRank returns the ranking of the player for the lifetime balance.
func lifetimeRank(s *discordgo.Session, i *discordgo.InteractionCreate) {
	log.Trace("--> leaderboard")
	defer log.Trace("<-- leaderboard")
}

// Start intializes the economy.
func Start(session *discordgo.Session) {
	LoadBanks()
	go resetMonthlyLeaderboard()
}

// GetCommands returns the component handlers, command handlers, and commands for the payday bot.
func GetCommands() (map[string]func(s *discordgo.Session, i *discordgo.InteractionCreate), map[string]func(s *discordgo.Session, i *discordgo.InteractionCreate), []*discordgo.ApplicationCommand) {
	commands := make([]*discordgo.ApplicationCommand, 0, len(memberCommands)+len(adminCommands))
	commands = append(commands, memberCommands...)
	commands = append(commands, adminCommands...)
	return nil, commandHandlers, commands
}
