package main

import (
	"encoding/json"
	"fmt"
	"net/http"
	"os"
	"os/signal"
	"strings"
	"syscall"

	"github.com/bwmarrin/discordgo"
	"github.com/summrs-dev-team/summrs-premium/api"
)

func main() {
	token := strings.TrimSpace(os.Getenv("DISCORD_TOKEN"))
	if token == "" {
		fmt.Print("Enter your token: ")
		fmt.Scan(&token)
	}

	req, _ := http.NewRequest("GET", "https://discord.com/api/v10/gateway/bot", nil)
	req.Header.Add("Authorization", fmt.Sprintf("Bot %s", token))

	res, err := http.DefaultClient.Do(req)
	if err != nil {
		fmt.Printf("[Sharding Error]: %s\n", err.Error())
		return
	}
	defer res.Body.Close()

	gresponse := &discordgo.GatewayBotResponse{}
	if err := json.NewDecoder(res.Body).Decode(gresponse); err != nil {
		fmt.Printf("[Decode Error]: %s\n", err.Error())
		return
	}

	shardCount := gresponse.Shards
	if shardCount < 1 {
		shardCount = 1
	}

	bot := api.Bot{Sessions: make([]*discordgo.Session, shardCount)}
	for shardID := 0; shardID < shardCount; shardID++ {
		bot.Shard(token, shardCount, shardID)
	}
	bot.Run()

	sc := make(chan os.Signal, 1)
	signal.Notify(sc, syscall.SIGINT, syscall.SIGTERM, os.Interrupt)
	<-sc

	bot.Stop()
}
