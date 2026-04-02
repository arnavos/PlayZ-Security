package commands

import (
	"bufio"
	"fmt"
	"math/rand"
	"os"
	"runtime"
	"strconv"
	"strings"
	"time"

	"github.com/summrs-dev-team/summrs-premium/events"
	"github.com/summrs-dev-team/summrs-premium/utils"

	"github.com/bwmarrin/discordgo"
)

func (cmd *Commands) BotInfo(s *discordgo.Session, m *discordgo.Message, ctx *Context) {
	uptime := time.Since(botStartedAt).Round(time.Second)
	heartbeat := s.HeartbeatLatency().Round(1 * time.Millisecond)
	ramLine := "RAM: `N/A`"
	if usedMB, totalMB, ok := getSystemRAMUsageMB(); ok && totalMB > 0 {
		ramLine = fmt.Sprintf("RAM: `%d/%d MB`", usedMB, totalMB)
	}
	cpuLine := "CPU: `N/A`"
	if cpuPct, ok := getSystemCPUUsagePercent(); ok {
		cpuLine = fmt.Sprintf("CPU: `%.1f%%`", cpuPct)
	}

	s.ChannelMessageSendEmbed(m.ChannelID, &discordgo.MessageEmbed{
		Title:       "PlayZ Stats",
		Description: "Overall stats for PlayZ Security",
		Fields: []*discordgo.MessageEmbedField{
			{
				Name: "General",
				Value: strings.Join([]string{
					fmt.Sprintf("Shards: `%d`", s.ShardCount),
					fmt.Sprintf("Servers: `%d`", events.GuildCount),
					fmt.Sprintf("Users: `%d`", events.MemberCount),
					fmt.Sprintf("Uptime: `%s`", uptime),
				}, "\n"),
				Inline: true,
			},
			{
				Name: "System",
				Value: strings.Join([]string{
					ramLine,
					cpuLine,
					fmt.Sprintf("Ping: `%s`", heartbeat),
					fmt.Sprintf("Go: `%s`", runtime.Version()),
				}, "\n"),
				Inline: true,
			},
			{
				Name: "Architecture",
				Value: strings.Join([]string{
					fmt.Sprintf("OS: `%s`", runtime.GOOS),
					fmt.Sprintf("Arch: `%s`", runtime.GOARCH),
					"Framework: `discordgo v0.29.0`",
					"Engine: `Go`",
				}, "\n"),
				Inline: true,
			},
		},
		Footer:    &discordgo.MessageEmbedFooter{Text: fmt.Sprintf("Requested by: %s", m.Author.Username)},
		Thumbnail: &discordgo.MessageEmbedThumbnail{URL: s.State.User.AvatarURL("500")},
		Color:     ThemeColor,
	})
}

func (cmd *Commands) Credits(s *discordgo.Session, m *discordgo.Message, ctx *Context) {
	s.ChannelMessageSendEmbed(m.ChannelID, &discordgo.MessageEmbed{
		Title: "🙌 Credits",
		Fields: []*discordgo.MessageEmbedField{
			{Name: "Creators:", Value: "[!fishgang Cy](https://github.com/Not-Cyrus) - Rewrote it in golang\n[lxi](https://github.com/lxi1400) - Made original bot/ bot hoster\n[four](https://tenor.com/view/bearded-bear-guy-slay-gay-pride-super-gay-lgbt-gif-16465293) - bot owner (lxi bb)\n[jinx](https://google.com)  - bot owner"},
		},
		Footer: &discordgo.MessageEmbedFooter{Text: fmt.Sprintf("Requested by: %s", m.Author.Username)},
		Color:  ThemeColor,
	})
}

func (cmds *Commands) Fox(s *discordgo.Session, m *discordgo.Message, ctx *Context) {
	rand.Seed(time.Now().Unix())

	resBody, err := utils.MakeRequest("GET", "https://raw.githubusercontent.com/Not-Cyrus/fox-pic-repo/main/count.txt", "", nil)
	if err != nil {
		s.ChannelMessageSend(m.ChannelID, "Error: could not fetch the amount of fox pics, try re-running the command.")
		return
	}

	maxcount, _ := strconv.Atoi(strings.TrimSuffix(string(resBody), "\n"))

	s.ChannelMessageSend(m.ChannelID, fmt.Sprintf("https://raw.githubusercontent.com/Not-Cyrus/fox-pic-repo/main/%d.jpg", rand.Intn(maxcount-0)+0))
}

func (cmd *Commands) Invite(s *discordgo.Session, m *discordgo.Message, ctx *Context) {

	s.ChannelMessageSendEmbed(m.ChannelID, &discordgo.MessageEmbed{
		Fields: []*discordgo.MessageEmbedField{
			{Name: "🔗 Bot Invite", Value: fmt.Sprintf("[Click Here](https://discord.com/api/oauth2/authorize?client_id=%s&permissions=8&scope=bot)", s.State.User.ID), Inline: true},
		},
		Footer: &discordgo.MessageEmbedFooter{Text: fmt.Sprintf("Requested by: %s", m.Author.Username)},
		Color:  ThemeColor,
	})
}

func (cmd *Commands) Ping(s *discordgo.Session, m *discordgo.Message, ctx *Context) {
	latencyMs := s.HeartbeatLatency().Milliseconds()

	s.ChannelMessageSendEmbed(m.ChannelID, &discordgo.MessageEmbed{
		Title:       "Pong <a:dots:1489200629036617750>",
		Description: fmt.Sprintf("`%d` ms", latencyMs),
		Thumbnail:   &discordgo.MessageEmbedThumbnail{URL: "https://cdn.discordapp.com/emojis/885681753593872455.gif?size=2048"},
		Color:       ThemeColor,
	})
}

var botStartedAt = time.Now()

func getSystemRAMUsageMB() (usedMB int, totalMB int, ok bool) {
	file, err := os.Open("/proc/meminfo")
	if err != nil {
		return 0, 0, false
	}
	defer file.Close()

	var totalKB, availableKB int64
	scanner := bufio.NewScanner(file)
	for scanner.Scan() {
		line := scanner.Text()
		if strings.HasPrefix(line, "MemTotal:") {
			fields := strings.Fields(line)
			if len(fields) >= 2 {
				totalKB, _ = strconv.ParseInt(fields[1], 10, 64)
			}
		}
		if strings.HasPrefix(line, "MemAvailable:") {
			fields := strings.Fields(line)
			if len(fields) >= 2 {
				availableKB, _ = strconv.ParseInt(fields[1], 10, 64)
			}
		}
	}

	if totalKB <= 0 || availableKB < 0 {
		return 0, 0, false
	}

	used := totalKB - availableKB
	return int(used / 1024), int(totalKB / 1024), true
}

func getSystemCPUUsagePercent() (float64, bool) {
	idle1, total1, ok := readCPUStat()
	if !ok {
		return 0, false
	}
	time.Sleep(200 * time.Millisecond)
	idle2, total2, ok := readCPUStat()
	if !ok {
		return 0, false
	}

	deltaIdle := idle2 - idle1
	deltaTotal := total2 - total1
	if deltaTotal <= 0 {
		return 0, false
	}

	usage := (float64(deltaTotal-deltaIdle) / float64(deltaTotal)) * 100
	if usage < 0 {
		usage = 0
	}
	if usage > 100 {
		usage = 100
	}

	return usage, true
}

func readCPUStat() (idle uint64, total uint64, ok bool) {
	file, err := os.Open("/proc/stat")
	if err != nil {
		return 0, 0, false
	}
	defer file.Close()

	scanner := bufio.NewScanner(file)
	if !scanner.Scan() {
		return 0, 0, false
	}

	fields := strings.Fields(scanner.Text())
	if len(fields) < 5 || fields[0] != "cpu" {
		return 0, 0, false
	}

	values := make([]uint64, 0, len(fields)-1)
	for _, part := range fields[1:] {
		v, parseErr := strconv.ParseUint(part, 10, 64)
		if parseErr != nil {
			return 0, 0, false
		}
		values = append(values, v)
		total += v
	}

	idle = values[3]
	if len(values) > 4 {
		idle += values[4]
	}
	return idle, total, true
}
