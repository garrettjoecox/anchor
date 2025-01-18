package main

import (
	"bufio"
	"bytes"
	"fmt"
	"log"
	"os"
	"os/signal"
	"runtime"
	"strconv"
	"strings"
	"syscall"
)

func main() {
	server := NewServer()

	errChan := make(chan error)
	sigsCa := make(chan os.Signal, 1)
	signal.Notify(sigsCa, syscall.SIGINT, syscall.SIGTERM)

	go func() {
		<-sigsCa
		signal.Stop(sigsCa)
		log.Println("Shutting down server...")
		buf := make([]byte, 1<<20)
		stacklen := runtime.Stack(buf, true)
		log.Printf("=== Goroutine Dump ===\n%s\n=== End ===", buf[:stacklen])

		server.saveStats()
		server.listener.Close()
		os.Exit(0)
	}()

	go func() {
		// Shut down server on first error
		if err := <-errChan; err != nil {
			log.Printf("Server shutting down due to error: %v", err)
			server.saveStats()
			server.listener.Close()
			os.Exit(1) // Exit the program
		}
	}()

	go processStdin(server)

	server.Start(errChan)
}

func getMessage(input []string) string {
	var message bytes.Buffer

	for i := 0; i <= len(input)-1; i++ {
		if i < len(input) {
			message.WriteString(input[i] + " ")
		} else {
			message.WriteString(input[i])
		}
	}

	return message.String()
}

func sendDisable(client *Client, message string) {
	sendServerMessage(client, message)
	client.sendPacket(`{"type":"DISABLE_ANCHOR"}`)
	client.disconnect()
}

func sendServerMessage(client *Client, message string) {
	if message == "" {
		message = "You have been disconnected by the server. Try to connect again in a bit!"
	}
	client.sendPacket(`{"type":"SERVER_MESSAGE","message":"` + message + `"}`)
}

func getClientID(clientID string) uint64 {
	converted, err := strconv.ParseUint(clientID, 10, 64)
	if err != nil {
		log.Println("Given text was not a valid clientID.")
		return 0
	}

	return converted
}

func processStdin(s *Server) {
	var reader bufio.Reader = *bufio.NewReader(os.Stdin)
	for {
		input, err := reader.ReadString('\n')

		if err != nil {
			log.Println("Error reading from stdin:", err)
			continue
		}

		// remove new line delimiter
		input = strings.Replace(input, "\n", "", 1)

		// split on space
		splitInput := strings.Split(input, " ")

		switch splitInput[0] {
		case "roomCount":
			var roomCount int
			s.rooms.Range(func(_, _ interface{}) bool {
				roomCount++
				return true
			})
			log.Println("Room count:", roomCount)
		case "clientCount":
			var clientCount int
			s.onlineClients.Range(func(_, _ interface{}) bool {
				clientCount++
				return true
			})
		case "quiet":
			s.quietMode.Store(!s.quietMode.Load())
			log.Println("Quiet mode:", s.quietMode.Load())
		case "stats":
			log.Println("Online Count:", strconv.FormatInt(int64(s.onlineCount()), 10), "| Total Games Complete: "+strconv.FormatInt(int64(s.totalGamesCompleteCount.Load()), 10), "| Monthly Games Complete: "+strconv.FormatInt(int64(s.monthlyGamesCompleteCount.Load()), 10))
			log.Println("Games Complete: " + strconv.FormatInt(int64(s.totalGamesCompleteCount.Load()), 10))
		case "list":
			s.rooms.Range(func(_, value interface{}) bool {
				room := value.(*Room)
				log.SetFlags(0)
				log.Println("Room", room.id+":")
				room.clients.Range(func(_, value interface{}) bool {
					client := value.(*Client)
					client.mu.Lock()
					log.Println("  Client", fmt.Sprint(client.id)+":", client.state)
					client.mu.Unlock()
					return true
				})
				log.SetFlags(log.LstdFlags)
				return true
			})
		case "disable":
			targetClientID := getClientID(splitInput[1])
			if targetClientID == 0 {
				continue
			}

			value, ok := s.onlineClients.Load(targetClientID)

			if ok {
				client := value.(*Client)
				log.Println("[Server] DISABLE_ANCHOR packet ->", client.id)
				go sendDisable(client, getMessage(splitInput[2:]))
			} else {
				log.Println("Client", targetClientID, "not found")
			}
		case "disableAll":
			log.Println("[Server] DISABLE_ANCHOR packet -> All")
			s.onlineClients.Range(func(_, value interface{}) bool {
				client := value.(*Client)
				go sendDisable(client, getMessage(splitInput[1:]))
				return true
			})
		case "message":
			targetClientID := getClientID(splitInput[1])
			if targetClientID == 0 {
				continue
			}

			value, ok := s.onlineClients.Load(targetClientID)

			if ok {
				client := value.(*Client)
				log.Println("[Server] SERVER_MESSAGE packet ->", client.id)
				go sendServerMessage(client, getMessage(splitInput[2:]))
			} else {
				log.Println("Client", targetClientID, "not found")
			}
		case "messageAll":
			log.Println("[Server] SERVER_MESSAGE packet -> All")
			s.onlineClients.Range(func(_, value interface{}) bool {
				client := value.(*Client)
				go sendServerMessage(client, getMessage(splitInput[1:]))
				return true
			})
		case "deleteRoom":
			targetRoomID := splitInput[1]

			_, ok := s.rooms.Load(targetRoomID)

			if ok {
				s.onlineClients.Range(func(_, value interface{}) bool {
					client := value.(*Client)
					if client.room.id == targetRoomID {
						go sendDisable(client, "Deleting your room. Goodbye!")
					}
					return true
				})
				s.rooms.Delete(targetRoomID)
			} else {
				log.Println("Room", targetRoomID, "not found")
			}

			s.mu.Unlock()
		case "banClient":
			targetClientID := getClientID(splitInput[1])
			value, ok := s.onlineClients.Load(targetClientID)

			client := value.(*Client)

			if ok {
				go func() {
					client.mu.Lock()
					conn := client.conn
					client.mu.Unlock()
					s.banIP(s.getSHA(conn))

					s.handleBannedConnection(client.conn)
				}()
			} else {
				log.Println("Client", targetClientID, "not found")
			}
		case "getClientSHA":
			targetClientID := getClientID(splitInput[1])
			value, ok := s.onlineClients.Load(targetClientID)

			client := value.(*Client)

			if ok {

				client.mu.Lock()
				conn := client.conn
				client.mu.Unlock()

				log.SetFlags(0)
				log.Println("Clients IP SHA: " + s.getSHA(conn))
				log.SetFlags(log.LstdFlags)
			} else {
				log.Println("Client", targetClientID, "not found")
			}
		case "banIP":
			s.banIP(splitInput[1])
		case "unbanIP":
			s.unbanIP(splitInput[1])
		case "unbanAll":
			s.mu.Lock()
			s.banList = nil
			s.mu.Unlock()
		case "stop":
			s.onlineClients.Range(func(_, value interface{}) bool {
				client := value.(*Client)
				go sendServerMessage(client, "Server restarting. Check back in a bit!")
				return true
			})

			s.saveStats()
			s.listener.Close()

			os.Exit(0)
		default:
			log.SetFlags(0)
			log.Println("Available commands:")
			log.Println("help: Show this help message")
			log.Println("stats: Print server stats")
			log.Println("quiet: Toggle quiet mode")
			log.Println("roomCount: Show the number of rooms")
			log.Println("clientCount: Show the number of clients")
			log.Println("list: List all rooms and clients")
			log.Println("stop <message>: Stop the server")
			log.Println("message <clientId> <message>: Send a message to a client")
			log.Println("messageAll <message>: Send a message to all clients")
			log.Println("disable <clientId> <message>: Disable anchor on a client")
			log.Println("disableAll <message>: Disable anchor on all clients")
			log.Println("deleteRoom <roomID>: Disables anchor on all online clients in the room and deletes it")
			log.Println("banIP <ip>:Adds an IP address to the ban list")
			log.Println("unbanIP <ip>:Removes an IP address from the ban list")
			log.Println("unbanAll: Unbans all IP addresses that are currently banned")
			log.Println("banClient <clientId>: Bans the IP of the selected Client and boots them")
			log.Println("getClientSHA <clientId>: Gets the client's IP SHA value")
			log.SetFlags(log.LstdFlags)
		}
	}
}
