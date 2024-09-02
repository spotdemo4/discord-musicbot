package main

import (
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"log"
	"os"
	"os/exec"
	"os/signal"
	"path/filepath"
	"strings"
	"time"

	"github.com/bwmarrin/discordgo"
	"github.com/spf13/viper"
)

var commands = []*discordgo.ApplicationCommand{
	{
		Name:        "play",
		Description: "Play a song from a URL",
		Options: []*discordgo.ApplicationCommandOption{
			{
				Name:        "url",
				Description: "URL of the song to play",
				Type:        discordgo.ApplicationCommandOptionString,
				Required:    true,
			},
		},
	},
	{
		Name:        "search",
		Description: "Search for a song on YouTube",
		Options: []*discordgo.ApplicationCommandOption{
			{
				Name:        "query",
				Description: "Search query",
				Type:        discordgo.ApplicationCommandOptionString,
				Required:    true,
			},
		},
	},
	{
		Name:        "queue",
		Description: "View the current song queue",
	},
	{
		Name:        "skip",
		Description: "Skip the current song",
	},
}

func main() {
	// Check if yt-dlp is installed
	if _, err := exec.LookPath("yt-dlp"); err != nil {
		log.Fatalf("yt-dlp is not installed")
	}

	// Check if ffmpeg is installed
	if _, err := exec.LookPath("ffmpeg"); err != nil {
		log.Fatalf("ffmpeg is not installed")
	}

	// Check if ffprobe is installed
	if _, err := exec.LookPath("ffprobe"); err != nil {
		log.Fatalf("ffprobe is not installed")
	}

	// Read in environment variables
	env := env{}
	if err := env.read(); err != nil {
		log.Fatalf("could not read environment variables: %s", err)
	}

	// Create a new Discord session using the provided bot token.
	session, err := discordgo.New("Bot " + env.DiscordToken)
	if err != nil {
		log.Fatalf("could not create session: %s", err)
	}

	// Add command handlers
	session.AddHandler(func(s *discordgo.Session, i *discordgo.InteractionCreate) {
		switch i.Type {

		case discordgo.InteractionApplicationCommand:
			data := i.ApplicationCommandData()
			switch data.Name {

			case "play":
				handlePlay(s, i, data.Options[0].StringValue())

			case "search":
				handleSearch(s, i, data.Options[0].StringValue())

			case "queue":
				handleQueue(s, i)

			case "skip":
				handleSkip(s, i)

			default:
				log.Printf("unknown command: %s", data.Name)

			}

		case discordgo.InteractionMessageComponent:
			data := i.MessageComponentData()
			switch data.CustomID {

			case "search":
				handlePlay(s, i, data.Values[0])

			default:
				log.Printf("unknown message component: %s", data.CustomID)

			}
		}
	})

	// Add ready handler
	session.AddHandler(func(s *discordgo.Session, r *discordgo.Ready) {
		log.Printf("Logged in as %s", r.User.String())

		for _, g := range r.Guilds {
			// Register commands
			_, err = session.ApplicationCommandBulkOverwrite(env.DiscordApplicationID, g.ID, commands)
			if err != nil {
				log.Printf("could not register commands for guild %s: %s", g.ID, err)
			}
		}
	})

	// Add on join guild handler
	session.AddHandler(func(s *discordgo.Session, e *discordgo.GuildCreate) {
		// Register commands
		_, err = session.ApplicationCommandBulkOverwrite(env.DiscordApplicationID, e.Guild.ID, commands)
		if err != nil {
			log.Printf("could not register commands for guild %s: %s", e.Guild.ID, err)
		}
	})

	session.Identify.Intents = discordgo.IntentsGuilds | discordgo.IntentsGuildMessages | discordgo.IntentsGuildVoiceStates

	// Open the websocket connection to Discord and begin listening.
	err = session.Open()
	if err != nil {
		log.Fatalf("could not open session: %s", err)
	}

	// Wait here until CTRL-C or other term signal is received.
	sigch := make(chan os.Signal, 1)
	signal.Notify(sigch, os.Interrupt)
	<-sigch

	err = session.Close()
	if err != nil {
		log.Printf("could not close session gracefully: %s", err)
	}
}

type env struct {
	// Discord API token
	DiscordToken         string `mapstructure:"DISCORD_TOKEN"`
	DiscordApplicationID string `mapstructure:"DISCORD_APPLICATION_ID"`
}

func (e *env) read() error {
	// Get config directory
	userConfigDir, err := os.UserConfigDir()
	if err != nil {
		return err
	}

	configDir := filepath.Join(userConfigDir, "discord-musicbot")

	// Check if config directory exists
	if _, err := os.Stat(configDir); os.IsNotExist(err) {
		// Create config directory
		if err := os.MkdirAll(configDir, 0755); err != nil {
			return errors.New("could not create config directory")
		}
	}

	// Check if env file exists
	if _, err := os.Stat(filepath.Join(configDir, "config.env")); os.IsNotExist(err) {
		// Create env file
		file, err := os.Create(filepath.Join(configDir, "config.env"))
		if err != nil {
			return errors.New("could not create config.env file")
		}
		defer file.Close()
	}

	// Set env file name and path
	viper.SetConfigName("config.env")
	viper.AddConfigPath(configDir)
	viper.SetConfigType("env")

	// Read in env file
	if err := viper.ReadInConfig(); err != nil {
		return err
	}

	// Read in environment variables
	if err := viper.BindEnv("DISCORD_TOKEN"); err != nil {
		return err
	}
	if err := viper.BindEnv("DISCORD_APPLICATION_ID"); err != nil {
		return err
	}

	// Unmarshal env variables
	if err := viper.Unmarshal(e); err != nil {
		return err
	}

	// Check if Discord token is set
	if e.DiscordToken == "" {
		return errors.New("DISCORD_TOKEN is not set")
	}
	if e.DiscordApplicationID == "" {
		return errors.New("DISCORD_APPLICATION_ID is not set")
	}

	return nil
}

var songQueues []*songQueue

type songQueue struct {
	GuildID string
	Songs   []*song
}

// handlePlay handles the play command
func handlePlay(s *discordgo.Session, i *discordgo.InteractionCreate, url string) {
	if err := s.InteractionRespond(i.Interaction, &discordgo.InteractionResponse{
		Type: discordgo.InteractionResponseDeferredChannelMessageWithSource,
		Data: &discordgo.InteractionResponseData{
			Flags: 1 << 6,
		},
	}); err != nil {
		log.Printf("could not respond to interaction: %s", err)
	}

	hexName := hex.EncodeToString([]byte(url))

	currentSong := &song{
		FileName: hexName,
		Url:      url,
	}

	// Download the song
	log.Printf("Downloading song: %s", currentSong.Url)
	if err := currentSong.download(); err != nil {
		if err := qResponse(s, i.Interaction, fmt.Sprintf("could not download song: %s", err)); err != nil {
			log.Printf("could not respond to interaction: %s", err)
		}

		return
	}

	// Convert the song
	log.Printf("Converting song: %s", *currentSong.FullFileName)
	if err := currentSong.convert(); err != nil {
		if err := qResponse(s, i.Interaction, fmt.Sprintf("could not convert song: %s", err)); err != nil {
			log.Printf("could not respond to interaction: %s", err)
		}

		return
	}

	// Load the song into memory
	log.Printf("Loading song: %s", *currentSong.FullFileName)
	if err := currentSong.load(); err != nil {
		if err := qResponse(s, i.Interaction, fmt.Sprintf("could not load song: %s", err)); err != nil {
			log.Printf("could not respond to interaction: %s", err)
		}

		return
	}

	// Find the channel the message came from
	channel, err := s.State.Channel(i.ChannelID)
	if err != nil {
		log.Printf("could not get channel: %s", err)
		return
	}

	// Find the guild for that channel
	guild, err := s.State.Guild(channel.GuildID)
	if err != nil {
		log.Printf("could not get guild: %s", err)
		return
	}

	// Look for the message sender in that guild's current voice states.
	for _, vs := range guild.VoiceStates {
		if vs.UserID == i.Member.User.ID {
			// Get song queue for guild
			currentSongQueue := songQueueByGuildID(guild.ID)

			// Create song queue if it doesn't exist
			if currentSongQueue == nil {
				currentSongQueue = &songQueue{
					GuildID: guild.ID,
					Songs:   []*song{currentSong},
				}
				songQueues = append(songQueues, currentSongQueue)
			} else {
				// Add song to queue
				currentSongQueue.Songs = append(currentSongQueue.Songs, currentSong)
			}

			// Play the song if it's the only song in the queue
			if len(currentSongQueue.Songs) > 1 {
				if err := qResponse(s, i.Interaction, fmt.Sprintf("%s added to queue", *currentSong.Name)); err != nil {
					log.Printf("could not respond to interaction: %s", err)
				}
			} else {
				if err := qResponse(s, i.Interaction, fmt.Sprintf("Playing %s", *currentSong.Name)); err != nil {
					log.Printf("could not respond to interaction: %s", err)
				}

				err = playSongQueue(s, guild.ID, vs.ChannelID)
				if err != nil {
					log.Printf("Error playing sound: %s", err)
				}
			}

			return
		}
	}

	resp := "You must be in a voice channel to use this command!"
	if _, err := s.InteractionResponseEdit(i.Interaction, &discordgo.WebhookEdit{
		Content: &resp,
	}); err != nil {
		log.Printf("could not respond to interaction: %s", err)
	}
}

// playSound plays the current buffer to the provided channel.
func playSongQueue(s *discordgo.Session, guildID, channelID string) (err error) {

	// Join the provided voice channel.
	vc, err := s.ChannelVoiceJoin(guildID, channelID, false, true)
	if err != nil {
		return err
	}

	// Sleep for a specified amount of time before playing the sound
	time.Sleep(250 * time.Millisecond)

	// Start speaking.
	vc.Speaking(true)

	// Start the loop for sending the buffer data.
	currentSongQueue := songQueueByGuildID(guildID)
	for len(currentSongQueue.Songs) > 0 {
		song := currentSongQueue.Songs[0]
		currentLength := len(currentSongQueue.Songs)

		// Update the game status
		s.UpdateGameStatus(0, *song.Name)

		// Send the buffer data.
		for _, buff := range *song.Buffer {
			vc.OpusSend <- buff

			// Skips the rest of the loop if the song is skipped
			if currentLength > len(currentSongQueue.Songs) {
				break
			} else if currentLength < len(currentSongQueue.Songs) {
				currentLength = len(currentSongQueue.Songs)
			}
		}

		if currentLength != len(currentSongQueue.Songs) {
			continue
		}

		// Remove the song from the queue
		currentSongQueue.Songs = currentSongQueue.Songs[1:]
	}

	// Update the game status
	s.UpdateGameStatus(0, "")

	// Stop speaking
	vc.Speaking(false)

	// Sleep for a specificed amount of time before ending.
	time.Sleep(250 * time.Millisecond)

	// Disconnect from the provided voice channel.
	vc.Disconnect()

	return nil
}

func handleSearch(s *discordgo.Session, i *discordgo.InteractionCreate, query string) {
	if err := s.InteractionRespond(i.Interaction, &discordgo.InteractionResponse{
		Type: discordgo.InteractionResponseDeferredChannelMessageWithSource,
		Data: &discordgo.InteractionResponseData{
			Flags: 1 << 6,
		},
	}); err != nil {
		log.Printf("could not respond to interaction: %s", err)
	}

	// Get search results
	results, err := search(query)
	if err != nil {
		if err := qResponse(s, i.Interaction, fmt.Sprintf("could not search for song: %s", err)); err != nil {
			log.Printf("could not respond to interaction: %s", err)
		}

		return
	}

	// Create select menu options
	var options []discordgo.SelectMenuOption
	for _, result := range results {
		options = append(options, discordgo.SelectMenuOption{
			Label:       result.Title,
			Value:       result.URL,
			Description: result.Uploader,
		})
	}

	// Create response
	value := 1
	if _, err := s.InteractionResponseEdit(i.Interaction, &discordgo.WebhookEdit{
		Components: &[]discordgo.MessageComponent{
			discordgo.ActionsRow{
				Components: []discordgo.MessageComponent{
					discordgo.SelectMenu{
						MenuType:    discordgo.StringSelectMenu,
						CustomID:    "search",
						Options:     options,
						Placeholder: "Select a song",
						MinValues:   &value,
						MaxValues:   value,
					},
				},
			},
		},
	}); err != nil {
		log.Printf("could not respond to interaction: %s", err)
	}
}

type searchResult struct {
	Title    string `json:"fulltitle"`
	URL      string `json:"webpage_url"`
	Uploader string `json:"uploader"`
}

// search searches for a song on YouTube, getting the first 5 results
func search(query string) ([]searchResult, error) {

	// Run the command
	cmd := exec.Command("yt-dlp", "--default-search", "ytsearch5", "-O", "%(.{fulltitle,webpage_url,uploader})j", query)
	output, err := cmd.Output()
	if err != nil {
		return []searchResult{}, err
	}

	// Parse the output
	var results []searchResult
	lines := strings.Split(string(output), "\n")
	for _, line := range lines {
		if line == "" {
			continue
		}

		var result searchResult
		if err := json.Unmarshal([]byte(line), &result); err != nil {
			return []searchResult{}, err
		}

		results = append(results, result)
	}

	return results, nil
}

// handleQueue handles the queue command
func handleQueue(s *discordgo.Session, i *discordgo.InteractionCreate) {
	if err := s.InteractionRespond(i.Interaction, &discordgo.InteractionResponse{
		Type: discordgo.InteractionResponseDeferredChannelMessageWithSource,
		Data: &discordgo.InteractionResponseData{
			Flags: 1 << 6,
		},
	}); err != nil {
		log.Printf("could not respond to interaction: %s", err)
	}

	// Get song queue for guild
	currentSongQueue := songQueueByGuildID(i.GuildID)
	if currentSongQueue == nil {
		if err := qResponse(s, i.Interaction, "No songs in queue"); err != nil {
			log.Printf("could not respond to interaction: %s", err)
		}
		return
	}

	// Create fields
	var fields []*discordgo.MessageEmbedField
	for i, song := range currentSongQueue.Songs {
		fields = append(fields, &discordgo.MessageEmbedField{
			Name:   fmt.Sprintf("%d. %s", i+1, *song.Name),
			Value:  song.Url,
			Inline: false,
		})
	}

	// Create response
	if _, err := s.InteractionResponseEdit(i.Interaction, &discordgo.WebhookEdit{
		Embeds: &[]*discordgo.MessageEmbed{
			{
				Title:  "Song Queue",
				Fields: fields,
			},
		},
	}); err != nil {
		log.Printf("could not respond to interaction: %s", err)
	}
}

// handleSkip handles the skip command
func handleSkip(s *discordgo.Session, i *discordgo.InteractionCreate) {
	if err := s.InteractionRespond(i.Interaction, &discordgo.InteractionResponse{
		Type: discordgo.InteractionResponseDeferredChannelMessageWithSource,
		Data: &discordgo.InteractionResponseData{
			Flags: 1 << 6,
		},
	}); err != nil {
		log.Printf("could not respond to interaction: %s", err)
	}

	// Get song queue for guild
	currentSongQueue := songQueueByGuildID(i.GuildID)
	if currentSongQueue == nil {
		if err := qResponse(s, i.Interaction, "No songs in queue"); err != nil {
			log.Printf("could not respond to interaction: %s", err)
		}
		return
	}

	skippedSongName := currentSongQueue.Songs[0].Name

	// Remove the first song from the queue
	currentSongQueue.Songs = currentSongQueue.Songs[1:]

	// Create response
	if err := qResponse(s, i.Interaction, fmt.Sprintf("%s skipped", *skippedSongName)); err != nil {
		log.Printf("could not respond to interaction: %s", err)
	}
}

// gets the song queue for a guild
func songQueueByGuildID(guildID string) *songQueue {
	for _, q := range songQueues {
		if q.GuildID == guildID {
			return q
		}
	}

	return nil
}

// quickly responds to an interaction
func qResponse(s *discordgo.Session, i *discordgo.Interaction, response string) error {
	if _, err := s.InteractionResponseEdit(i, &discordgo.WebhookEdit{
		Content: &response,
	}); err != nil {
		log.Printf("could not respond to interaction: %s", err)
	}

	return nil
}
