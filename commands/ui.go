package commands

import (
	"fmt"
	"time"

	"github.com/bwmarrin/discordgo"
)

func sendStyledEmbed(s *discordgo.Session, channelID, title, description string, color int, author string) {
	embed := &discordgo.MessageEmbed{
		Title:       title,
		Description: description,
		Color:       color,
		Timestamp:   time.Now().Format(time.RFC3339),
	}
	if author != "" {
		embed.Footer = &discordgo.MessageEmbedFooter{Text: fmt.Sprintf("Requested by: %s", author)}
	}
	_, _ = s.ChannelMessageSendEmbed(channelID, embed)
}

func sendError(s *discordgo.Session, channelID, description, author string) {
	sendStyledEmbed(s, channelID, "❌ Error", description, 0xEF4444, author)
}

func sendSuccess(s *discordgo.Session, channelID, description, author string) {
	sendStyledEmbed(s, channelID, "✅ Success", description, 0x22C55E, author)
}

func sendInfo(s *discordgo.Session, channelID, title, description, author string) {
	sendStyledEmbed(s, channelID, title, description, ThemeColor, author)
}
