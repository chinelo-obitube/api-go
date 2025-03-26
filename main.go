package main

import (
	"context"
	"encoding/json"
	"fmt"
	"log"
	"net/http"
	"os"

	"github.com/gorilla/mux"
	"github.com/joho/godotenv"
	"github.com/machinebox/graphql"
)

const newRelicGraphQLEndpoint = "https://api.eu.newrelic.com/graphql"

// request
type InsertKeyRequest struct {
	AccountID  int    `json:"account_id"`
	Name       string `json:"name"`
	Notes      string `json:"notes"`
	IngestType string `json:"ingestType"`
}

// response
type NewRelicResponse struct {
	APIAccessCreateKeys struct {
		CreatedKeys []struct {
			ID         string `json:"id"`
			Key        string `json:"key"`
			Name       string `json:"name"`
			Notes      string `json:"notes"`
			Type       string `json:"type"`
			IngestType string `json:"ingestType"`
		} `json:"createdKeys"`
		Errors []struct {
			Message    string `json:"message"`
			Type       string `json:"type"`
			AccountID  int    `json:"accountId"`
			ErrorType  string `json:"errorType"`
			IngestType string `json:"ingestType"`
		} `json:"errors"`
	} `json:"apiAccessCreateKeys"`
}

type Server struct {
	client *graphql.Client
}

func (s *Server) createApiKey(w http.ResponseWriter, r *http.Request) {
	log.Println("Received request to create a new key")

	apiKey := os.Getenv("NEW_RELIC_API_KEY")
	if apiKey == "" {
		http.Error(w, `{"error": "Missing NEW_RELIC_API_KEY"}`, http.StatusUnauthorized)
		return
	}

	// Parse request body
	var request InsertKeyRequest
	err := json.NewDecoder(r.Body).Decode(&request)
	if err != nil {
		http.Error(w, `{"error": "Invalid JSON request body"}`, http.StatusBadRequest)
		return
	}

	mutation := fmt.Sprintf(`
        mutation {
            apiAccessCreateKeys(
                keys: {
                    ingest: {
                        accountId: %d
                        ingestType: %s
                        name: "%s"
                        notes: "%s"
                    }
                }
            ) {
                createdKeys {
                    id
                    key
                    name
                    notes
                    type
                    ... on ApiAccessIngestKey {
                        ingestType
                    }
                }
                errors {
                    message
                    type
                    ... on ApiAccessIngestKeyError {
                        accountId
                        errorType
                        ingestType
                    }
                }
            }
        }
    `, request.AccountID, request.IngestType, request.Name, request.Notes)

	req := graphql.NewRequest(mutation)

	req.Header.Set("API-Key", apiKey)
	req.Header.Set("Content-Type", "application/json")

	ctx := context.Background()
	var responseData NewRelicResponse

	// Executing the GraphQL request
	err = s.client.Run(ctx, req, &responseData)
	if err != nil {
		log.Printf("Error executing GraphQL request: %v", err)
		http.Error(w, fmt.Sprintf(`{"error": "Failed to create insert key", "details": "%s"}`, err.Error()), http.StatusInternalServerError)
		return
	}

	// Check if any keys were created
	if len(responseData.APIAccessCreateKeys.CreatedKeys) > 0 {
		createdKey := responseData.APIAccessCreateKeys.CreatedKeys[0]
		log.Printf("Successfully created key: ID=%s, Name=%s", createdKey.ID, createdKey.Name)
		w.WriteHeader(http.StatusOK)
		json.NewEncoder(w).Encode(map[string]any{
			"insert_key": createdKey,
		})
		return
	}

	// Handle API errors if no keys were created
	if len(responseData.APIAccessCreateKeys.Errors) > 0 {
		// log.Printf("API returned errors: %v", responseData.APIAccessCreateKeys.Errors)
		http.Error(w, fmt.Sprintf("API returned an error: %v", responseData.APIAccessCreateKeys.Errors), http.StatusBadRequest)
		return
	}

	// If no keys and no errors, return a generic error
	log.Println("No keys were created and no errors were returned by the API")
	http.Error(w, "No key was created", http.StatusInternalServerError)
}

func main() {
	err := godotenv.Load()
	if err != nil {
		log.Fatal("Error loading .env file")
	}

	client := graphql.NewClient(newRelicGraphQLEndpoint)
	server := &Server{client: client}

	r := mux.NewRouter()
	r.HandleFunc("/create-insert-key", server.createApiKey).Methods("POST")

	port := ":8080"
	fmt.Println("Server is running on port", port)
	log.Fatal(http.ListenAndServe(port, r))
}
