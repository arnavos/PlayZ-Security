package commands

import (
	"fmt"
	"strconv"

	"github.com/bwmarrin/discordgo"
	"github.com/summrs-dev-team/summrs-premium/utils"
)

func (cmd *Commands) Avatar(s *discordgo.Session, m *discordgo.Message, ctx *Context) {
	target := m.Author
	if len(m.Mentions) > 0 {
		target = m.Mentions[0]
	}
	avatarURL := target.AvatarURL("4096")

	s.ChannelMessageSendEmbed(m.ChannelID, &discordgo.MessageEmbed{
		Description: fmt.Sprintf("[Image URL](%s)", avatarURL),
		Image:       &discordgo.MessageEmbedImage{URL: avatarURL},
		Footer:      &discordgo.MessageEmbedFooter{Text: fmt.Sprintf("Requested by: %s", m.Author.Username)},
		Color:       ThemeColor,
	})
}

func (cmd *Commands) UserInfo(s *discordgo.Session, m *discordgo.Message, ctx *Context) {
	member, err := s.GuildMember(m.GuildID, m.Mentions[0].ID)
	if err != nil {
		return
	}

	var (
		memberCreatedAt, _ = discordgo.SnowflakeTimestamp(m.Mentions[0].ID)
		memberJoinedAt     = member.JoinedAt
		role               = utils.HighestRole(s, m.GuildID, member)
		roleID             = "@everyone"
	)

	if role != nil {
		roleID = fmt.Sprintf("<@&%s>", role.ID)
	}

	s.ChannelMessageSendEmbed(m.ChannelID, &discordgo.MessageEmbed{
		Author:    &discordgo.MessageEmbedAuthor{Name: fmt.Sprintf("👤 User info for: %s#%s", m.Mentions[0].Username, m.Mentions[0].Discriminator)},
		Thumbnail: &discordgo.MessageEmbedThumbnail{URL: m.Mentions[0].AvatarURL("1024")},

		Fields: []*discordgo.MessageEmbedField{
			{Name: "🧠 Username:", Value: m.Mentions[0].Username, Inline: true},
			{Name: "📅 Account Made On:", Value: memberCreatedAt.Format("01/02/2006"), Inline: true},
			{Name: "📥 Account Joined On:", Value: memberJoinedAt.Format("01/02/2006"), Inline: true},
			{Name: "🤖 Bot?", Value: strconv.FormatBool(m.Mentions[0].Bot), Inline: true},
			{Name: "🛡️ Highest Role:", Value: roleID, Inline: true},
			{Name: "🟢 Status", Value: "Coming back later.", Inline: true},
		},

		Footer: &discordgo.MessageEmbedFooter{Text: fmt.Sprintf("Requested by: %s", m.Author.Username)},
		Color:  ThemeColor,
	})
}
