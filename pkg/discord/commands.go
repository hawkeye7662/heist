package discord

import (
	"github.com/bwmarrin/discordgo"
	"github.com/rbrabson/heist/pkg/msg"
	log "github.com/sirupsen/logrus"
)

var (
	helpCommandHandler = map[string]func(s *discordgo.Session, i *discordgo.InteractionCreate){
		"help":      help,
		"adminhelp": adminHelp,
	}

	helpCommands = []*discordgo.ApplicationCommand{
		{

			Name:        "help",
			Description: "Provides a description of commands for this server",
		},
		{
			Name:        "adminhelp",
			Description: "Provides a description of admin commands for this server",
		},
	}
)

func help(s *discordgo.Session, i *discordgo.InteractionCreate) {
	log.Debug("--> help")
	log.Debug("<-- help")

	msg.SendResponse(s, i, getMemberHelp())
}

func adminHelp(s *discordgo.Session, i *discordgo.InteractionCreate) {
	log.Debug("--> adminHelp")
	log.Debug("<-- adminHelp")

	msg.SendResponse(s, i, getAdminHelp())
}