package main

import (
	"log"
	"sync"

	"github.com/tidwall/gjson"
)

type Team struct {
	id                       string
	clientIdsRequestingState []uint64
	room                     *Room
	state                    string     // Save state
	queue                    []string   // Packet queue to apply to Save
	mu                       sync.Mutex // Mutex for safely updating state/queue
}

func (t *Team) broadcastPacket(packet string) {
	clientId := gjson.Get(packet, "clientId").Uint()

	t.room.clients.Range(func(_, value interface{}) bool {
		client := value.(*Client)
		if client.team == t && client.conn != nil && client.id != clientId {
			go func(c *Client) {
				defer func() {
					if r := recover(); r != nil {
						log.Printf("Panic in sendPacket for client %d: %v", c.id, r)
					}
				}()
				c.sendPacket(packet)
			}(client)
		}

		return true
	})
}
