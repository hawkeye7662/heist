package msg

import (
	"github.com/bwmarrin/discordgo"
	log "github.com/sirupsen/logrus"
)

// SendEphemeralResponse is a utility routine used to send an ephemeral response to a user's message or button press.
func SendNonephemeralResponse(s *discordgo.Session, i *discordgo.InteractionCreate, msg string) {
	log.Debug("--> SendNonephemeralResponse")
	defer log.Debug("<-- SendNonephemeralResponse")

	err := s.InteractionRespond(i.Interaction, &discordgo.InteractionResponse{
		Type: discordgo.InteractionResponseChannelMessageWithSource,
		Data: &discordgo.InteractionResponseData{
			Content: msg,
		},
	})
	if err != nil {
		log.Error("Unable to send a response, error:", err)
	}
}

// SendEphemeralResponse is a utility routine used to send an ephemeral response to a user's message or button press.
func SendEphemeralResponse(s *discordgo.Session, i *discordgo.InteractionCreate, msg string) {
	log.Debug("--> SendEphemeralResponse")
	defer log.Debug("<-- SendEphemeralResponse")

	err := s.InteractionRespond(i.Interaction, &discordgo.InteractionResponse{
		Type: discordgo.InteractionResponseChannelMessageWithSource,
		Data: &discordgo.InteractionResponseData{
			Content: msg,
			Flags:   discordgo.MessageFlagsEphemeral,
		},
	})
	if err != nil {
		log.Error("Unable to send a response, error:", err)
	}
}
