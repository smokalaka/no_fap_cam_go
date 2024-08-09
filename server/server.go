package main

import (
	"encoding/json"
	"log"
	"net/http"
	"sync"

	"github.com/google/uuid"
	"github.com/gorilla/websocket"
)

type Client struct {
	UserId string
	UUID   string
	Conn   *websocket.Conn
	Peer   *Client
}

type RegisterRequest struct {
	UserId string `json:"userId"`
}

type RegisterResponse struct {
	UUID string `json:"uuid"`
}

type Offer struct {
	Type string `json:"type"`
	SDP  string `json:"sdp"`
}

type Answer struct {
	Type   string `json:"type"`
	Answer string `json:"answer"`
}

type ICECandidate struct {
	Type      string `json:"type"`
	Candidate string `json:"candidate"`
}

type Server struct {
	clients map[string]*Client
	mu      sync.Mutex
	pool    []*Client
}

func (s *Server) registerHandler(w http.ResponseWriter, r *http.Request) {
	if r.Method == http.MethodOptions {
		w.Header().Set("Access-Control-Allow-Origin", "*")
		w.Header().Set("Access-Control-Allow-Methods", "POST, OPTIONS")
		w.Header().Set("Access-Control-Allow-Headers", "Content-Type")
		w.WriteHeader(http.StatusOK)
		return
	}

	w.Header().Set("Access-Control-Allow-Origin", "*")
	w.Header().Set("Content-Type", "application/json")

	decoder := json.NewDecoder(r.Body)
	var registerRequest RegisterRequest
	err := decoder.Decode(&registerRequest)
	if err != nil {
		http.Error(w, "Invalid request body", http.StatusBadRequest)
		return
	}

	uuid := uuid.NewString()

	s.mu.Lock()
	client := &Client{UserId: registerRequest.UserId, UUID: uuid}
	s.clients[uuid] = client
	s.pool = append(s.pool, client)
	s.mu.Unlock()

	response := RegisterResponse{UUID: uuid}
	jsonResponse, err := json.Marshal(response)
	if err != nil {
		http.Error(w, "Error marshalling response", http.StatusInternalServerError)
		return
	}

	w.Header().Set("Content-Type", "application/json")
	w.Write(jsonResponse)
}

func (s *Server) pairClients() {
	for len(s.pool) >= 2 {
		client1 := s.pool[0]
		client2 := s.pool[1]
		s.pool = s.pool[2:]

		if client1 != nil && client2 != nil {
			client1.Peer = client2
			client2.Peer = client1

			log.Printf("Pairing clients %s: %s", client1.UUID, client2.UUID)
			// telling client1 to send offer
			client1.Conn.WriteJSON(map[string]string{"type": "send_offer"})
			// client2 would send answer after receiving offer from client1
		}
	}
}

var upgrader = websocket.Upgrader{
	CheckOrigin: func(r *http.Request) bool {
		return true
	},
}

func (s *Server) connectHandler(w http.ResponseWriter, r *http.Request) {
	uuid := r.URL.Query().Get("uuid")
	if uuid == "" {
		http.Error(w, "UUID is required", http.StatusBadRequest)
		return
	}

	conn, err := upgrader.Upgrade(w, r, nil)
	if err != nil {
		log.Println("Upgrade error:", err)
		return
	}

	s.mu.Lock()
	// TODO: Check if no client is found with this uuid
	client := s.clients[uuid]
	client.Conn = conn
	s.pairClients()
	s.mu.Unlock()

	go s.handleMessages(client)
}

func (s *Server) handleMessages(client *Client) {
	defer client.Conn.Close()

	for {
		_, msg, err := client.Conn.ReadMessage()
		if err != nil {
			log.Println("Read error:", err)
			break
		}
		log.Printf("Received message from %s: %s", client.UUID, msg)

		var offer Offer
		var answer Answer
		var ice ICECandidate

		if err := json.Unmarshal(msg, &offer); err == nil && offer.Type == "offer" {
			// TODO: Only forward message when still connected
			s.forwardMessage(client.Peer, msg)
		} else if err := json.Unmarshal(msg, &answer); err == nil && answer.Type == "answer" {
			s.forwardMessage(client.Peer, msg)
		} else if err := json.Unmarshal(msg, &ice); err == nil && ice.Type == "ice" {
			// TODO: Only forward message when still connected
			s.forwardMessage(client.Peer, msg)
		}
	}
}

func (s *Server) forwardMessage(client *Client, msg []byte) {
	if client != nil {
		client.Conn.WriteMessage(websocket.TextMessage, msg)
	}
}

func main() {
	log.Println("Starting server ....")
	s := Server{
		clients: make(map[string]*Client),
	}

	http.HandleFunc("/register", s.registerHandler)
	http.HandleFunc("/connect", s.connectHandler)

	go func() {
		err := http.ListenAndServe(":8080", nil)
		if err != nil {
			log.Fatalf("Error while starting server: %v", err)
		}
	}()
	log.Println("Server listening on port 8080")
	select {}
}
