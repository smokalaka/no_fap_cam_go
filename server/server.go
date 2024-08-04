package main

import (
	"encoding/json"
	"log"
	"net/http"

	"github.com/google/uuid"
	"github.com/gorilla/websocket"
)

type Client struct {
	UUID string `json:"uuid"`
}

type RegisterRequest struct {
	UserId string `json:"userId"`
}

type RegisterResponse struct {
	UUID string `json:"uuid"`
}

type Server struct {
	clients map[string]Client
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
	s.clients[registerRequest.UserId] = Client{UUID: uuid}

	response := RegisterResponse{UUID: uuid}
	jsonResponse, err := json.Marshal(response)
	if err != nil {
		http.Error(w, "Error marshalling response", http.StatusInternalServerError)
		return
	}

	w.Header().Set("Content-Type", "application/json")
	w.Write(jsonResponse)
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

	//	s.clients[uuid] = Client{UUID: uuid}

	go func() {
		defer conn.Close()
		for {
			_, msg, err := conn.ReadMessage()
			if err != nil {
				log.Println("Read error:", err)
				break
			}
			log.Printf("Received message from %s: %s", uuid, msg)

			err = conn.WriteMessage(websocket.TextMessage, msg)
			if err != nil {
				log.Println("Write error:", err)
				break
			}
		}
	}()
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
