package main

import (
	"bufio"
	"bytes"
	"fmt"
	"log"
	"os"
	"os/signal"
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
		fmt.Println("Shutting down server...")
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
		fmt.Println("Given text was not a valid clientID.")
		return 0
	}

	return converted
}

func processStdin(s *Server) {
	var reader bufio.Reader = *bufio.NewReader(os.Stdin)
	for {
		input, err := reader.ReadString('\n')

		if err != nil {
			fmt.Println("Error reading from stdin:", err)
			continue
		}

		// remove new line delimiter
		input = strings.Replace(input, "\n", "", 1)

		// split on space
		splitInput := strings.Split(input, " ")

		switch splitInput[0] {
		case "roomCount":
			fmt.Println("Room count: ", len(s.rooms))
		case "clientCount":
			fmt.Println("Client count:", len(s.onlineClients))
		case "quiet":
			s.quietMode = !s.quietMode
			fmt.Println("Quiet mode: ", s.quietMode)
		case "stats":
			s.mu.Lock()
			fmt.Println("Current Stats:")
			fmt.Println("    Online Count: " + strconv.FormatInt(int64(len(s.onlineClients)), 10))
			fmt.Println("    Games Complete: " + strconv.FormatInt(int64(s.gamesCompleted), 10))
			s.mu.Unlock()
		case "list":
			for _, room := range s.rooms {
				fmt.Println("Room", room.id+":")
				for _, client := range room.clients {
					fmt.Println("  Client", fmt.Sprint(client.id)+":", client.state)
				}
			}
		case "disable":
			targetClientId := getClientID(splitInput[1])
			if targetClientId == 0 {
				continue
			}

			client := s.onlineClients[targetClientId]

			if client != nil {
				fmt.Println("[Server] DISABLE_ANCHOR packet ->", client.id)
				go sendDisable(client, getMessage(splitInput[2:]))
				continue
			}

			fmt.Println("Client", targetClientId, "not found")
		case "disableAll":
			fmt.Println("[Server] DISABLE_ANCHOR packet -> All")
			for i := range s.onlineClients {
				client := s.onlineClients[i]
				go sendDisable(client, getMessage(splitInput[1:]))
			}
		case "message":
			targetClientId := getClientID(splitInput[1])
			if targetClientId == 0 {
				continue
			}

			client := s.onlineClients[targetClientId]

			if client != nil {
				fmt.Println("[Server] SERVER_MESSAGE packet ->", client.id)
				go sendServerMessage(client, getMessage(splitInput[2:]))
				continue
			}

			fmt.Println("Client", targetClientId, "not found")
		case "messageAll":
			fmt.Println("[Server] SERVER_MESSAGE packet -> All")
			s.mu.Lock()
			for _, client := range s.onlineClients {
				go sendServerMessage(client, getMessage(splitInput[1:]))
			}
			s.mu.Unlock()
		case "deleteRoom":
			targetRoomID := splitInput[1]
			s.rooms[targetRoomID].mu.Lock()
			for _, client := range s.onlineClients {
				if client.room.id == targetRoomID {
					go sendDisable(client, "Deleting your room. Goodbye!")
				}
			}
			s.rooms[targetRoomID].mu.Unlock()
			delete(s.rooms, targetRoomID)
		case "stop":
			s.mu.Lock()
			for _, client := range s.onlineClients {
				go sendServerMessage(client, "Server restarting. Check back in a bit!")
			}
			s.mu.Unlock()

			s.saveStats()
			s.listener.Close()

			os.Exit(0)
		default:
			fmt.Printf("Available commands:\nhelp: Show this help message\nstats: Print server stats\nquiet: Toggle quiet mode\nroomCount: Show the number of rooms\nclientCount: Show the number of clients\nlist: List all rooms and clients\nstop <message>: Stop the server\nmessage <clientId> <message>: Send a message to a client\nmessageAll <message>: Send a message to all clients\ndisable <clientId> <message>: Disable anchor on a client\ndisableAll <message>: Disable anchor on all clients\ndeleteRoom <roomID>: Disables anchor on all online clients in the room and deletes it\n")
		}
	}
}
