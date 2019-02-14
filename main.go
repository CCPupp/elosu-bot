package main

import (
	"database/sql"
	"flag"
	"fmt"
	"lib"
	"os"
	"os/signal"
	"strings"
	"syscall"
	"time"

	_ "github.com/bmizerany/pq"

	"github.com/bwmarrin/discordgo"
)

var (
	// Token export
	Token string
)

const (
	host     = lib.ElosuHost
	port     = lib.ElosuPort
	user     = lib.ElosuUser
	password = lib.ElosuPassword
	dbname   = lib.ElosuDbname
)

func init() {

	flag.StringVar(&Token, "t", "", "Bot Token")
	flag.Parse()
}

func main() {
	dg, err := discordgo.New("Bot " + lib.ElosuBotToken)
	if err != nil {
		fmt.Println("error creating Discord session,", err)
		return
	}

	// Register the messageCreate func as a callback for MessageCreate events.
	dg.AddHandler(messageCreate)

	// Open a websocket connection to Discord and begin listening.
	err = dg.Open()
	if err != nil {
		fmt.Println("error opening connection,", err)
		return
	}

	// Wait here until CTRL-C or other term signal is received.
	fmt.Println("Bot is now running.  Press CTRL-C to exit.")
	sc := make(chan os.Signal, 1)
	signal.Notify(sc, syscall.SIGINT, syscall.SIGTERM, os.Interrupt, os.Kill)
	<-sc

	// Cleanly close down the Discord session.
	dg.Close()
}

// This function will be called (due to AddHandler above) every time a new
// message is created on any channel that the autenticated bot has access to.
func messageCreate(s *discordgo.Session, m *discordgo.MessageCreate) {

	// Ignore all messages created by the bot itself
	// This isn't required in this specific example but it's a good practice.
	if m.Author.ID == s.State.User.ID {
		return
	}
	// If the message starts with trigger
	if strings.HasPrefix(m.Content, "e!") {

		// If the message is "ping" reply with "Pong!"
		if strings.HasSuffix(m.Content, "ping") {
			s.ChannelMessageSend(m.ChannelID, "Pong!")
		}

		// If the message is "pong" reply with "Ping!"
		if strings.HasSuffix(m.Content, "pong") {
			s.ChannelMessageSend(m.ChannelID, "Ping!")
		}

		if strings.HasSuffix(m.Content, "stats") {
			var message string
			message = getUserInfo(m.Author.ID)
			s.ChannelMessageSend(m.ChannelID, message)
		}
	}

}

func getUserInfo(id string) string {

	psqlInfo := fmt.Sprintf("host=%s port=%d user=%s "+
		"password=%s dbname=%s sslmode=disable",
		host, port, user, password, dbname)
	db, err := sql.Open("postgres", psqlInfo)
	checkErr(err)
	defer db.Close()
	//Pings to check the connection
	err = db.Ping()
	checkErr(err)

	//Test to read the users from the db
	userinf, err := db.Query("SELECT * FROM player WHERE discid = $1", id)
	checkErr(err)

	var out string

	var (
		playerid int
		name     string
		elo      int
		wins     int
		losses   int
		joindate time.Time
		discid   int
	)
	for userinf.Next() {
		err = userinf.Scan(&playerid, &name, &elo, &wins, &losses, &joindate, &discid)
		checkErr(err)
	}

	out = fmt.Sprintf("User: %v \nElo: %v \nWins: %v ", name, elo, wins)

	return out

}

func checkExisting(id string) bool {

	psqlInfo := fmt.Sprintf("host=%s port=%d user=%s "+
		"password=%s dbname=%s sslmode=disable",
		host, port, user, password, dbname)
	db, err := sql.Open("postgres", psqlInfo)
	checkErr(err)
	defer db.Close()
	//Pings to check the connection
	err = db.Ping()
	checkErr(err)

	//Test to read the users from the db
	userinf, err := db.Query("SELECT name FROM player WHERE discid = $1", id)
	checkErr(err)

	var out string

	var (
		name string
	)
	for userinf.Next() {
		err = userinf.Scan(&name)
		checkErr(err)
	}

	out = fmt.Sprintf("User: %v", name)
	if out == "" {
		return true
	}

	return false

}

func newUser(id, name, elo, discordID string) {

	psqlInfo := fmt.Sprintf("host=%s port=%d user=%s "+
		"password=%s dbname=%s sslmode=disable",
		host, port, user, password, dbname)
	db, err := sql.Open("postgres", psqlInfo)
	if err != nil {
		panic(err)
	}
	defer db.Close()
	//Pings to check the connection
	err = db.Ping()
	if err != nil {
		panic(err)
	}

	//Test to add my user to the db
	_, err = db.Exec("INSERT INTO player (playerid, name, elo, wins, losses, joindate, discid) VALUES VALUES($1, $2, $3, 0, 0, current_timestamp, $4)",
		id, name, elo, discordID)

	checkErr(err)

}

//Stops bot and displays error in console
func checkErr(err error) {
	if err != nil {
		panic(err)
	}
}
