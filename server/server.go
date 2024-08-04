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
	To   string `json:"to"`
	From string `json:"from"`
}

type ICECandidate struct {
	Type      string `json:"type"`
	Candidate string `json:"candidate"`
	To        string `json:"to"`
	From      string `json:"from"`
}

type Server struct {
	clients map[string]Client
	mu      sync.Mutex
	pool    []string
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
	s.clients[uuid] = Client{UserId: registerRequest.UserId, UUID: uuid}
	s.pool = append(s.pool, uuid)
	s.pairClients()
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
		client1UUID := s.pool[0]
		client2UUID := s.pool[1]
		s.pool = s.pool[2:]

		// client1 := s.clients[client1UUID]
		// client2 := s.clients[client2UUID]

		log.Printf("Pairing clients %s: %s", client1UUID, client2UUID)
		// see if we need to coordinate message passing b/w them
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
	client := s.clients[uuid]
	client.Conn = conn
	s.clients[uuid] = client
	s.mu.Unlock()

	go s.handleMessages(client)
}

func (s *Server) handleMessages(client Client) {
	defer client.Conn.Close()

	for {
		_, msg, err := client.Conn.ReadMessage()
		if err != nil {
			log.Println("Read error:", err)
			break
		}
		log.Printf("Received message from %s: %s", client.UUID, msg)

		var offer Offer
		var ice ICECandidate

		if err := json.Unmarshal(msg, &offer); err == nil && offer.Type == "offer" {
			s.forwardMessage(offer.To, msg)
		} else if err := json.Unmarshal(msg, &ice); err == nil && ice.Type == "ice" {
			s.forwardMessage(ice.To, msg)
		}

		// echo. To be removed
		// err = conn.WriteMessage(websocket.TextMessage, msg)
		// if err != nil {
		// 	log.Println("Write error:", err)
		// 	break
		// }
	}
}

func (s *Server) forwardMessage(to string, msg []byte) {
	s.mu.Lock()
	client, exists := s.clients[to]
	s.mu.Unlock()
	if exists {
		client.Conn.WriteMessage(websocket.TextMessage, msg)
	}
}

func main() {
	log.Println("Starting server ....")
	s := Server{
		clients: make(map[string]Client),
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
