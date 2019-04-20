package main

import (
	"encoding/json"
	"log"
	"math/rand"
	"strconv"
	"time"

	"github.com/gorilla/websocket"
)

type Client struct {
	conn *websocket.Conn

	// sendCallback is callback based on packetID
	sendCallback map[string]func(req WSPacket)
	// recvCallback is callback when receive based on ID of the packet
	recvCallback map[string]func(req WSPacket)
}

type WSPacket struct {
	ID   string `json:"id"`
	Data string `json:"data"`

	RoomID      string `json:"room_id"`
	PlayerIndex int    `json:"player_index"`

	TargetHostID string `json:"target_id"`
	PacketID     string `json:"packet_id"`
	// Globally ID of a session
	SessionID string `json:"session_id"`
}

var EmptyPacket = WSPacket{}

func NewClient(conn *websocket.Conn) *Client {
	sendCallback := map[string]func(WSPacket){}
	recvCallback := map[string]func(WSPacket){}
	return &Client{
		conn: conn,

		sendCallback: sendCallback,
		recvCallback: recvCallback,
	}
}

// send sends a packet and trigger callback when the packet comes back
func (c *Client) send(request WSPacket, callback func(response WSPacket)) {
	request.PacketID = strconv.Itoa(rand.Int())
	data, err := json.Marshal(request)
	if err != nil {
		return
	}

	c.conn.WriteMessage(websocket.TextMessage, data)
	wrapperCallback := func(resp WSPacket) {
		resp.PacketID = request.PacketID
		resp.SessionID = request.SessionID
		callback(resp)
	}
	if callback == nil {
		return
	}
	c.sendCallback[request.PacketID] = wrapperCallback
}

// receive receive and response back
func (c *Client) receive(id string, f func(response WSPacket) (request WSPacket)) {
	c.recvCallback[id] = func(response WSPacket) {
		req := f(response)
		// Add Meta data
		req.PacketID = response.PacketID
		req.SessionID = response.SessionID

		// Skip rqeuest if it is EmptyPacket
		if req == EmptyPacket {
			return
		}
		resp, err := json.Marshal(req)
		if err != nil {
			log.Println("[!] json marshal error:", err)
		}
		c.conn.SetWriteDeadline(time.Now().Add(writeWait))
		c.conn.WriteMessage(websocket.TextMessage, resp)
	}
}

// syncSend sends a packet and wait for callback till the packet comes back
func (c *Client) syncSend(request WSPacket) (response WSPacket) {
	res := make(chan WSPacket)
	f := func(resp WSPacket) {
		res <- resp
	}
	c.send(request, f)
	return <-res
}

func (c *Client) listen() {
	for {
		log.Println("Waiting for message")
		_, rawMsg, err := c.conn.ReadMessage()
		if err != nil {
			log.Println("[!] read:", err)
			break
		}
		wspacket := WSPacket{}
		err = json.Unmarshal(rawMsg, &wspacket)
		if err != nil {
			continue
		}

		// Check if some async send is waiting for the response based on packetID
		if callback, ok := c.sendCallback[wspacket.PacketID]; ok {
			callback(wspacket)
			delete(c.sendCallback, wspacket.PacketID)
			// Skip receiveCallback to avoid duplication
			continue
		}
		// Check if some receiver with the ID is registered
		if callback, ok := c.recvCallback[wspacket.ID]; ok {
			callback(wspacket)
		}
	}
}