package main

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"log"
	"net/http"
	"os"
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/cemilick/go-dev-tools/db"
	"github.com/cemilick/go-dev-tools/ws"
	"github.com/google/uuid"
	"github.com/gorilla/mux"
	"github.com/joho/godotenv"
	"github.com/rs/cors"
	"go.mongodb.org/mongo-driver/bson"
	"go.mongodb.org/mongo-driver/mongo"
	"go.mongodb.org/mongo-driver/mongo/options"
)

// WebhookData represents a single webhook request
type WebhookData struct {
	ID          string                 `json:"id"`
	Timestamp   int64                  `json:"timestamp"`
	Method      string                 `json:"method"`
	Headers     map[string]string      `json:"headers"`
	Body        map[string]any `json:"body" bson:"body"`
	QueryParams map[string]string      `json:"queryParams"`
	UserAgent   string                 `json:"userAgent"`
	IP          string                 `json:"ip"`
	RawBody     string                 `json:"rawBody"`
}

// WebhookEndpoint represents a webhook endpoint
type WebhookEndpoint struct {
	ID        string         `json:"id"`
	URL       string         `json:"url"`
	CreatedAt int64          `json:"createdAt"`
	Requests  []*WebhookData `json:"requests"`
	BrowserID string         `json:"browserId"`
}

// CreateEndpointRequest represents the request to create an endpoint
type CreateEndpointRequest struct {
	CustomID  string `json:"customId,omitempty"`
	BrowserID string `json:"browserId"`
}

// CreateEndpointResponse represents the response after creating an endpoint
type CreateEndpointResponse struct {
	ID        string `json:"id"`
	URL       string `json:"url"`
	CreatedAt int64  `json:"createdAt"`
	Requests []*WebhookData `json:"requests,omitempty"` // Optional, can be empty
}

// Server holds our application state
type Server struct {
	endpoints map[string]*WebhookEndpoint
	mu        sync.RWMutex
	baseURL   string
	mongoClient *mongo.Client
	dbName    string
	hub 	 *ws.Hub
}

// NewServer creates a new server instance
func NewServer(baseURL string) *Server {
	return &Server{
		endpoints: make(map[string]*WebhookEndpoint),
		baseURL:   baseURL,
	}
}

var hub *ws.Hub

// generateUUIDv7 generates a UUID v7
func generateUUIDv7() string {
	return uuid.New().String()
}

// getClientIP extracts client IP from request
func getClientIP(r *http.Request) string {
	ip := r.Header.Get("X-Forwarded-For")
	if ip == "" {
		ip = r.Header.Get("X-Real-IP")
	}
	if ip == "" {
		ip = strings.Split(r.RemoteAddr, ":")[0]
	}
	return ip
}

// extractQueryParams converts URL query parameters to map
func extractQueryParams(r *http.Request) map[string]string {
	params := make(map[string]string)
	for key, values := range r.URL.Query() {
		if len(values) > 0 {
			params[key] = values[0] // Take first value if multiple
		}
	}
	return params
}

// extractHeaders converts HTTP headers to map
func extractHeaders(r *http.Request) map[string]string {
	headers := make(map[string]string)
	for key, values := range r.Header {
		if len(values) > 0 {
			headers[key] = values[0] // Take first value if multiple
		}
	}
	return headers
}

// CreateEndpoint creates a new webhook endpoint
func (s *Server) CreateEndpoint(w http.ResponseWriter, r *http.Request) {
	var req CreateEndpointRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		http.Error(w, "Invalid JSON", http.StatusBadRequest)
		return
	}

	if req.BrowserID == "" {
		http.Error(w, "Browser ID is required", http.StatusBadRequest)
		return
	}

	s.mu.Lock()
	defer s.mu.Unlock()

	// Generate ID
	var endpointID string
	if req.CustomID != "" {
		// Check in MongoDB if endpoint with this customID and browserID already exists
		ctx, cancel := context.WithTimeout(context.Background(), 3*time.Second)
		defer cancel()
		coll := s.mongoClient.Database(s.dbName).Collection("endpoints")
		count, err := coll.CountDocuments(ctx, bson.M{"id": req.CustomID})
		if err != nil {
			http.Error(w, "Failed to check custom ID", http.StatusInternalServerError)
			return
		}
		if count > 0 {
			http.Error(w, "Custom ID already exists", http.StatusConflict)
			return
		}
		endpointID = req.CustomID
	} else {
		endpointID = generateUUIDv7()
	}

	endpoint := &WebhookEndpoint{
		ID:        endpointID,
		URL:       fmt.Sprintf("https://tools.wazzi.site/webhook/%s", endpointID),
		CreatedAt: time.Now().Unix(),
		Requests:  make([]*WebhookData, 0),
		BrowserID: req.BrowserID,
	}

	// Simpan ke MongoDB
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	coll := s.mongoClient.Database(s.dbName).Collection("endpoints")
	_, err := coll.InsertOne(ctx, endpoint)
	if err != nil {
		http.Error(w, "Failed to save endpoint", http.StatusInternalServerError)
		return
	}

	s.endpoints[endpointID] = endpoint

	response := CreateEndpointResponse{
		ID:        endpoint.ID,
		URL:       endpoint.URL,
		CreatedAt: endpoint.CreatedAt,
		Requests:  endpoint.Requests,
	}

	wsPayload := map[string]any{
		"type":     "new-endpoint",
		"browserId": req.BrowserID,
		"endpoint": endpoint,
	}
	msg, _ := json.Marshal(wsPayload)
	s.hub.Broadcast <- msg

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(response)
}

// GetEndpoints returns all endpoints for a specific browser
func (s *Server) GetEndpoints(w http.ResponseWriter, r *http.Request) {
	browserID := r.URL.Query().Get("browserId")
	if browserID == "" {
		http.Error(w, "Browser ID is required", http.StatusBadRequest)
		return
	}

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	
	
	coll := s.mongoClient.Database(s.dbName).Collection("endpoints")
	
	// Delete endpoints older than 3 days
	threeDaysAgo := time.Now().Add(-72 * time.Hour).Unix()
	_, _ = coll.DeleteMany(ctx, bson.M{"createdat": bson.M{"$lt": threeDaysAgo}})
	
	cursor, err := coll.Find(ctx, bson.M{"browserid": browserID})

	if err != nil {
		http.Error(w, "Failed to get endpoints", http.StatusInternalServerError)
		return
	}
	defer cursor.Close(ctx)

	var endpoints []*WebhookEndpoint
	if err := cursor.All(ctx, &endpoints); err != nil {
		http.Error(w, "Failed to decode endpoints", http.StatusInternalServerError)
		return
	}

	// For each endpoint, fetch its latest webhook requests (e.g., up to 10)
	for _, endpoint := range endpoints {
		dataColl := s.mongoClient.Database(s.dbName).Collection(endpoint.ID)
		dataCursor, err := dataColl.Find(ctx, bson.M{}, options.Find().SetSort(bson.D{{Key: "timestamp", Value: -1}}).SetLimit(10))
		if err == nil {
			var requests []*WebhookData
			if err := dataCursor.All(ctx, &requests); err == nil {
				endpoint.Requests = requests
			}
			dataCursor.Close(ctx)
		}
	}

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(endpoints)
}



// ReceiveWebhook handles incoming webhook requests
func (s *Server) ReceiveWebhook(w http.ResponseWriter, r *http.Request) {
	vars := mux.Vars(r)
	endpointID := vars["id"]

	// Check if endpoint exists in MongoDB
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	endpointsColl := s.mongoClient.Database(s.dbName).Collection("endpoints")
	var endpoint WebhookEndpoint
	err := endpointsColl.FindOne(ctx, bson.M{"id": endpointID}).Decode(&endpoint)
	if err != nil {
		http.Error(w, "Endpoint not found", http.StatusNotFound)
		return
	}

	body, err := io.ReadAll(r.Body)
	if err != nil {
		http.Error(w, "Error reading body", http.StatusInternalServerError)
		return
	}

	var parsedBody map[string]any
	_ = json.Unmarshal(body, &parsedBody)

	webhookData := &WebhookData{
		ID:          generateUUIDv7(),
		Timestamp:   time.Now().UnixMilli(),
		Method:      r.Method,
		Headers:     extractHeaders(r),
		Body:        parsedBody,
		QueryParams: extractQueryParams(r),
		UserAgent:   r.UserAgent(),
		IP:          getClientIP(r),
		RawBody:     string(body),
	}

	// Save webhook data to a collection named after the endpointID
	dataColl := s.mongoClient.Database(s.dbName).Collection(endpointID)
	_, err = dataColl.InsertOne(context.Background(), webhookData)
	if err != nil {
		http.Error(w, "Failed to save webhook data", http.StatusInternalServerError)
		return
	}

	// Update in-memory cache (if present)
	s.mu.Lock()
	if memEndpoint, ok := s.endpoints[endpointID]; ok {
		memEndpoint.Requests = append([]*WebhookData{webhookData}, memEndpoint.Requests...)
		if len(memEndpoint.Requests) > 100 {
			memEndpoint.Requests = memEndpoint.Requests[:100]
		}
	}
	s.mu.Unlock()

	wsPayload := map[string]any{
		"type":     "incoming-webhook",
		"requestId": webhookData.ID,
		"browserId": endpoint.BrowserID,
		"data": webhookData,
	}
	msg, _ := json.Marshal(wsPayload)
	s.hub.Broadcast <- msg

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(map[string]any{
		"success":   true,
		"message":   "Webhook received successfully",
		"requestId": webhookData.ID,
		"timestamp": webhookData.Timestamp,
		"data":     webhookData,
	})
}


// GetWebhookData returns all webhook data for a specific endpoint
func (s *Server) GetWebhookData(w http.ResponseWriter, r *http.Request) {
	vars := mux.Vars(r)
	endpointID := vars["id"]

	limitStr := r.URL.Query().Get("limit")
	limit := 50
	if limitStr != "" {
		if l, err := strconv.Atoi(limitStr); err == nil && l > 0 && l <= 100 {
			limit = l
		}
	}

	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	coll := s.mongoClient.Database(s.dbName).Collection(endpointID)
	cursor, err := coll.Find(ctx, bson.M{}, options.Find().SetSort(bson.D{{Key: "timestamp", Value: -1}}).SetLimit(int64(limit)))
	if err != nil {
		http.Error(w, "Failed to get webhook data", http.StatusInternalServerError)
		return
	}
	
	var requests []WebhookData = make([]WebhookData, 0)
	if err := cursor.All(ctx, &requests); err != nil {
		log.Printf("Failed to decode webhook data: %v", err)
		http.Error(w, "Failed to decode webhook data", http.StatusInternalServerError)
		return
	}
	
	defer cursor.Close(ctx)

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(map[string]any{
		"endpointId": endpointID,
		"requests":   requests,
		"showing":    len(requests),
	})
}


// DeleteEndpoint deletes a webhook endpoint
func (s *Server) DeleteEndpoint(w http.ResponseWriter, r *http.Request) {
	vars := mux.Vars(r)
	endpointID := vars["id"]

	browserID := r.URL.Query().Get("browserId")
	if browserID == "" {
		http.Error(w, "Browser ID is required", http.StatusBadRequest)
		return
	}

	s.mu.Lock()
	defer s.mu.Unlock()

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	endpointsColl := s.mongoClient.Database(s.dbName).Collection("endpoints")
	var endpoint WebhookEndpoint
	err := endpointsColl.FindOne(ctx, bson.M{"id": endpointID}).Decode(&endpoint)
	if err != nil {
		http.Error(w, "Endpoint not found", http.StatusNotFound)

		return
	}

	if endpoint.BrowserID != browserID {
		http.Error(w, "Unauthorized", http.StatusForbidden)
		return
	}

	coll := s.mongoClient.Database(s.dbName).Collection("endpoints")
	_, errDel := coll.DeleteOne(ctx, bson.M{"id": endpointID})
	if errDel != nil {
		http.Error(w, "Failed to delete endpoint", http.StatusInternalServerError)
		return
	}

	// Opsional: hapus juga koleksi data webhook dengan nama endpointID jika ingin bersih total
	_ = s.mongoClient.Database(s.dbName).Collection(endpointID).Drop(ctx)

	delete(s.endpoints, endpointID)

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(map[string]any{
		"success": true,
		"message": "Endpoint deleted successfully",
	})
}


// HealthCheck returns server health status
func (s *Server) HealthCheck(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(map[string]any{
		"status":    "healthy",
		"timestamp": time.Now().Unix(),
		"version":   "1.0.0",
	})
}

func (s *Server) setupAPIRoutes() http.Handler {
    r := mux.NewRouter()

    api := r.PathPrefix("/api").Subrouter()
    api.HandleFunc("/health", s.HealthCheck).Methods("GET")
    api.HandleFunc("/endpoints", s.CreateEndpoint).Methods("POST")
    api.HandleFunc("/endpoints", s.GetEndpoints).Methods("GET")
    api.HandleFunc("/endpoints/{id}", s.DeleteEndpoint).Methods("DELETE")
    api.HandleFunc("/endpoints/{id}/data", s.GetWebhookData).Methods("GET")

    api.HandleFunc("/generate-token", GenerateTokenHandler).Methods("GET")
    api.HandleFunc("/generate-secret", GenerateSecretHandler).Methods("GET")
    api.HandleFunc("/generate-timestamp", GenerateTimestampHandler).Methods("GET")
    api.HandleFunc("/generate-password", GeneratePasswordHandler).Methods("GET")

    return r
}

// setupRoutes configures all API routes
func (s *Server) setupRoutes() http.Handler {
    r := mux.NewRouter()

    // Root frontend
    r.HandleFunc("/", IndexHandler).Methods("GET")

    // API routes under /api
    r.PathPrefix("/api").Handler(s.setupAPIRoutes())

    // Webhook receiver (accept any HTTP method)
    r.HandleFunc("/webhook/{id}", s.ReceiveWebhook).Methods("GET", "POST", "PUT", "DELETE", "PATCH", "HEAD", "OPTIONS")

    // WebSocket endpoint
    r.HandleFunc("/ws", func(w http.ResponseWriter, r *http.Request) {
        ws.ServeWS(s.hub, w, r)
    })

    // Setup CORS middleware on the entire router
    c := cors.New(cors.Options{
        AllowedOrigins:   []string{"https://webhook.wazzi.site"}, // ganti sesuai frontend mu
        AllowedMethods:   []string{"GET", "POST", "PUT", "DELETE", "PATCH", "HEAD", "OPTIONS"},
        AllowedHeaders:   []string{"Content-Type", "Authorization", "X-Requested-With"},
        AllowCredentials: true,
		Debug: true,
    })

    return c.Handler(r)
}

func main() {
	err := godotenv.Load()
	if err != nil {
		log.Fatal("Error loading .env file")
	}

	// SSH tunnel config
	sshUser := os.Getenv("SSH_USER")
	sshPass := os.Getenv("SSH_PASS")
	sshHost := os.Getenv("SSH_HOST")
	
	sshPort, err := strconv.Atoi(os.Getenv("SSH_PORT"))
	if err != nil {
		log.Fatal("Invalid SSH_PORT in .env")
	}

	// Local bind for tunnel (localhost:27018 misal)
	localAddr := os.Getenv("LOCAL_BIND")

	// Remote MongoDB host:port (server MongoDB)
	remoteAddr := os.Getenv("REMOTE_MONGO")

	// Start SSH Tunnel
	tunnel, err := db.NewTunnel(sshUser, sshPass, sshHost, sshPort, localAddr, remoteAddr)
	if err != nil {
		log.Fatalf("Failed to create SSH tunnel: %v", err)
	}
	defer tunnel.Close()
	log.Println("SSH tunnel established")

	// Connect to MongoDB via tunnel (gunakan localAddr sebagai host MongoDB)
	mongoURI := os.Getenv("MONGO_URI")
	mongoClient, err := db.ConnectMongo(mongoURI)
	if err != nil {
		log.Fatalf("Failed to connect MongoDB: %v", err)
	}
	log.Println("MongoDB connected")
	// Get port from environment or use default
    port := os.Getenv("PORT")
	if port == "" {
		port = "8080" // Default port
	}
    host := os.Getenv("HOST")

    baseURL := fmt.Sprintf("%s:%s", host, port)
	
	hub := ws.NewHub()
	go hub.Run()

	http.HandleFunc("/ws", func(w http.ResponseWriter, r *http.Request) {
		ws.ServeWS(hub, w, r)
	})

    // Create webhook server (assuming you have NewServer function)
    server := NewServer(baseURL)
    server.mongoClient = mongoClient
    server.dbName = "webhook"
    server.hub = hub

    handler := server.setupRoutes()

    http.Handle("/static/", http.StripPrefix("/static/", http.FileServer(http.Dir("static"))))
    http.Handle("/", handler)  

	// Logging
    log.Printf("🚀 Server starting on port %s", port)
    log.Printf("📡 Base URL: %s", baseURL)
    log.Printf("🏥 Health check: %s/api/health", baseURL)
    log.Printf("📖 API Documentation:")
    log.Printf("   Original endpoints:")
    log.Printf("   GET    %s/                       - Main page", baseURL)
    log.Printf("   POST   %s/generate-token         - Generate token", baseURL)
    log.Printf("   POST   %s/generate-secret        - Generate secret", baseURL)
    log.Printf("   POST   %s/generate-timestamp     - Generate timestamp", baseURL)
    log.Printf("   POST   %s/generate-password      - Generate password", baseURL)
    log.Printf("   Webhook endpoints:")
    log.Printf("   POST   %s/api/endpoints          - Create endpoint", baseURL)
    log.Printf("   GET    %s/api/endpoints          - Get endpoints by browser ID", baseURL)
    log.Printf("   DELETE %s/api/endpoints/{id}     - Delete endpoint", baseURL)
    log.Printf("   GET    %s/api/endpoints/{id}/data - Get webhook data", baseURL)
    log.Printf("   *      %s/webhook/{id}           - Receive webhooks", baseURL)

    if err := http.ListenAndServe(":"+port, nil); err != nil {
        log.Fatal("Server failed to start:", err)
    }
}