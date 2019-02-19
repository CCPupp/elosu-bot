package main

import (
	"database/sql"
	"flag"
	"fmt"
	"lib"
	"math"
	"os"
	"os/signal"
	"strconv"
	"strings"
	"syscall"
	"time"

	_ "github.com/bmizerany/pq"
	"github.com/bwmarrin/discordgo"
)

var (
	// Token export
	Token                        string
	pqueue                       = make(map[int]Player)
	player1elo, player1pc        int
	player2elo, player2pc        int
	winner, finallelo, finalwelo int
	name1, name2                 string
)

// Player object
type Player struct {
	Name string
	Elo  int
	Wait int
	ID   int
}

const (
	host     = lib.ElosuHost
	port     = lib.ElosuPort
	user     = lib.ElosuUser
	password = lib.ElosuPassword
	dbname   = lib.ElosuDbname
	token    = lib.ElosuBotToken
)

func init() {
	flag.StringVar(&Token, "t", "", "Bot Token")
	flag.Parse()
}

func main() {

	dg, err := discordgo.New("Bot " + token)
	if err != nil {
		fmt.Println("error creating Discord session,", err)
		return
	}
	go playerQueue(dg)

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

		if strings.HasSuffix(m.Content, "stats") {
			out := printUserInfo(m.Author.ID)
			s.ChannelMessageSend(m.ChannelID, out)
		}

		if strings.Contains(m.Content, "create") {
			if isExisting(m.Author.ID) {
				s.ChannelMessageSend(m.ChannelID, "You already have an account!")
			} else if strings.HasSuffix(m.Content, "create") {
				s.ChannelMessageSend(m.ChannelID, "Please add your osu account ID after the create command!")
			} else {
				str := strings.SplitAfterN(m.Content, " ", 2)
				if _, err := strconv.Atoi(str[1]); err == nil {
					newUser(str[1], m.Author.Username, m.Author.ID)
					s.ChannelMessageSend(m.ChannelID, "Your account has been created, if your username on osu is not the same as your discord username change it with \"e!namechange [username]\".")
				} else {
					s.ChannelMessageSend(m.ChannelID, "I don't think that is an ID, make sure to use your ID and not your username!")
				}

			}
		}

		if strings.Contains(m.Content, "namechange") {
			str := strings.SplitAfterN(m.Content, " ", 2)
			changeName(str[1], m.Author.ID)

			_, name, _, _, _, _ := getUserInfo(m.Author.ID)
			s.ChannelMessageSend(m.ChannelID, name)
		}

		if strings.Contains(m.Content, "join") {
			if isExisting(m.Author.ID) {
				_, name, elo, _, _, id := getUserInfo(m.Author.ID)
				newPlayer := Player{
					Name: name,
					Elo:  elo,
					Wait: 0,
					ID:   id,
				}
				addToQueue(newPlayer)
				s.ChannelMessageSend(m.ChannelID, "Added "+name+" to queue!")
			}
		}

		if strings.Contains(m.Content, "leave") {
			if isExisting(m.Author.ID) {
				_, name, elo, _, _, id := getUserInfo(m.Author.ID)
				newPlayer := Player{
					Name: name,
					Elo:  elo,
					Wait: 0,
					ID:   id,
				}
				removeFromQueue(newPlayer, false)
				s.ChannelMessageSend(m.ChannelID, "Removed "+name+" from the queue.")
			}
		}
		// ADMIN ONLY COMMANDS
		if checkAdmin(m.Author.ID) {
			if strings.Contains(m.Content, "queue") {
				out := "In queue:\n ```"
				for i := 0; i < len(pqueue); i++ {
					out += pqueue[i].Name + " | Waiting for: " + strconv.Itoa(pqueue[i].Wait) + "\n"
				}
				if len(pqueue) == 0 {
					out = "No one is in queue."
				} else {
					out += "```"
				}
				s.ChannelMessageSend(m.ChannelID, out)
			}

			if strings.Contains(m.Content, "submit") {
				str := strings.Split(m.Content, " ")
				matchResults(str[2], str[3])
				matchid, _ := strconv.Atoi(str[1])
				winnerid, _ := strconv.Atoi(str[2])
				loserid, _ := strconv.Atoi(str[3])
				winnerscore, _ := strconv.Atoi(str[4])
				loserscore, _ := strconv.Atoi(str[5])
				addMatchToDB(matchid, winnerid, loserid, winnerscore, loserscore)
			}
		}
	}
}

func checkAdmin(id string) bool {
	//Pupper, ode
	admins := map[string]bool{"98190856254676992": true, "98183996185255936": true}

	return admins[id]
}

func addToQueue(player Player) {
	pqueue[len(pqueue)] = player
}

func removeFromQueue(player Player, second bool) {
	for i := 0; i < len(pqueue); i++ {
		if second {
			i++
		}
		if pqueue[i].ID == player.ID {
			delete(pqueue, i)
		}
	}
}

func addMatchToDB(mid, wid, lid, wscore, lscore int) {
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
	_, err = db.Exec("INSERT INTO matches (matchid, wid, lid, wscore, lscore, date) VALUES ( $1, $2, $3, $4, $5, current_timestamp)", mid, wid, lid, wscore, lscore)
	checkErr(err)
}

func changeName(name, id string) {
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
	_, err = db.Exec("UPDATE player SET name = $1 WHERE discid = $2", name, id)
	checkErr(err)
}

func matchResults(idWin, idLose string) {
	var (
		welo, wwins, wlosses int
		lelo, lwins, llosses int
	)
	_, _, welo, wwins, wlosses, _ = getUserInfo(idWin)
	_, _, lelo, lwins, llosses, _ = getUserInfo(idLose)

	elow, elol := calcK(1, welo, lelo, (wwins + wlosses), (lwins + llosses))

	updatePlayerElo(elow, idWin, true)
	updatePlayerElo(elol, idLose, false)

}

func updatePlayerElo(elo int, id string, win bool) {
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

	if win {
		_, err = db.Exec("UPDATE player SET elo = $1, wins = wins + 1 WHERE discid = $2", strconv.Itoa(elo), id)
		checkErr(err)
	} else {
		_, err = db.Exec("UPDATE player SET elo = $1, losses = losses + 1 WHERE discid = $2", strconv.Itoa(elo), id)
		checkErr(err)
	}

}

func printUserInfo(id string) string {

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

	out = fmt.Sprintf("User: %v \nElo: %v \nWins: %v \n Join Date: %v ", name, elo, wins, joindate)

	return out

}

func getUserInfo(id string) (int, string, int, int, int, int) {

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

	return playerid, name, elo, wins, losses, discid

}

func isExisting(id string) bool {

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

	var (
		name string
	)
	for userinf.Next() {
		err = userinf.Scan(&name)
		checkErr(err)
	}

	if name == "" {
		return false
	}

	return true

}

func newUser(str, name, discordID string) {

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
	_, err = db.Exec("INSERT INTO player (playerid, name, elo, wins, losses, joindate, discid) VALUES ( $1, $2, 1200, 0, 0, current_timestamp, $3)", str, name, discordID)
	checkErr(err)

}

// Constantly running method that finds 2 players that have been added to the queue that are near enough to each other in rank and pairs them together.
func playerQueue(dg *discordgo.Session) {
	for true {

		if len(pqueue) >= 2 {
			for i := 0; i < len(pqueue); i++ {
				for j := 0; j < len(pqueue); j++ {
					//check if players are close enough to each other
					if inBetween(pqueue[j].Elo, pqueue[i].Elo-pqueue[i].Wait, pqueue[i].Elo+pqueue[i].Wait) && pqueue[i].ID != pqueue[j].ID {
						//return these 2 players
						dg.ChannelMessageSend("456253171220611082", "<@"+strconv.Itoa(pqueue[i].ID)+"> vs <@"+strconv.Itoa(pqueue[j].ID)+">")
						removeFromQueue(pqueue[i], false)
						removeFromQueue(pqueue[j], true)
					}
				}
			}
		}

		//increment all players wait by some value
		for i := 0; i < len(pqueue); i++ {
			var x = pqueue[i]
			x.Wait += 3
			pqueue[i] = x
		}

		//wait x seconds
		time.Sleep(3 * time.Second)
	}
}

//used to check if player is near another in elo
func inBetween(i, min, max int) bool {
	if (i >= min) && (i <= max) {
		return true
	}
	return false

}

// ********** START ELO CALCULATOR **********
// Does calculatons to find the correct elo of the 2 players and returns an HTML string with the new values
func calcElo(welo, lelo, wk, lk int) (int, int) {
	// Fuck what does this do again
	var wdiv = float64(float64(welo) / 400.0)
	var rw = float64(math.Pow(10, wdiv))
	var ldiv = float64(float64(lelo) / 400.0)
	var rl = float64(math.Pow(10, ldiv))
	finalwelo = welo + int(float64(wk)*(1-(rw/(rw+rl))))
	finallelo = lelo + int(float64(lk)*(0-(rl/(rw+rl))))
	//return formatted elo values
	return finalwelo, finallelo
}

// Calculates the K value and calls calcElo using the correct winner/loser ordering
func calcK(winner, player1elo, player2elo, player1pc, player2pc int) (int, int) {
	var wk, lk int
	var newK = 75
	var oldK = 25
	// Logic for faster new player growth
	if player1pc <= 10 {
		if winner == 1 {
			wk = newK
		} else {
			lk = newK
		}
	} else if player1pc > 10 {
		if winner == 1 {
			wk = oldK
		} else {
			lk = oldK
		}
	}
	if player2pc <= 10 {
		if winner == 2 {
			wk = newK
		} else {
			lk = newK
		}
	} else if player2pc > 10 {
		if winner == 2 {
			wk = oldK
		} else {
			lk = oldK
		}
	}
	// Choses correct order for player that won.
	if winner == 1 {
		w, l := calcElo(player1elo, player2elo, wk, lk)
		return w, l
	}
	w, l := calcElo(player2elo, player1elo, wk, lk)
	return w, l

}

// ********** STOP ELO CALCULATOR **********

//Stops bot and displays error in console
func checkErr(err error) {
	if err != nil {
		panic(err)
	}
}
