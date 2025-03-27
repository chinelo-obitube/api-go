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

// const newRelicGraphQLEndpoint = "https://api.eu.newrelic.com/graphql"

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

type DeleteKeyRequest struct {
	ID string `json:"id"`
}

type DeleteKeysResponse struct {
	ApiAccessDeleteKeys struct {
		DeletedKeys []struct {
			ID string `json:"id"`
		} `json:"deletedKeys"`
		Errors []struct {
			Message                 string `json:"message"`
			Type                    string `json:"type"`
			ApiAccessIngestKeyError struct {
				ID        string `json:"id"`
				Message   string `json:"message"`
				ErrorType string `json:"errorType"`
				AccountID string `json:"accountId"`
			} `json:"apiAccessIngestKeyError,omitempty"`
		} `json:"errors"`
	} `json:"apiAccessDeleteKeys"`
}

type DeleteKeysRequest struct {
	Keys struct {
		IngestKeyIDs string `json:"ingestKeyIds"`
	} `json:"keys"`
}

type Server struct {
	client *graphql.Client
	apiKey string
}

// Create an API key
func (s *Server) createApiKey(w http.ResponseWriter, r *http.Request) {
	log.Println("Received request to create a new key")

	var request InsertKeyRequest

	err := json.NewDecoder(r.Body).Decode(&request)
	if err != nil {
		log.Printf(`{"error": "Invalid JSON request body"}, Status Code: %d`, http.StatusBadRequest)
		http.Error(w, "Invalid JSON request body", http.StatusBadRequest)
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

	req.Header.Set("API-Key", s.apiKey)
	req.Header.Set("Content-Type", "application/json")

	ctx := context.Background()
	var responseData NewRelicResponse

	err = s.client.Run(ctx, req, &responseData)
	if err != nil {
		log.Printf("Failed to create insert key: %v, Status Code: %d", err, http.StatusInternalServerError)
		http.Error(w, "Failed to create insert key", http.StatusInternalServerError)
		return
	}

	if len(responseData.APIAccessCreateKeys.CreatedKeys) > 0 {
		createdKey := responseData.APIAccessCreateKeys.CreatedKeys[0]
		log.Printf("Successfully created key: ID=%s, Name=%s", createdKey.ID, createdKey.Name)
		w.WriteHeader(http.StatusOK)
		json.NewEncoder(w).Encode(map[string]any{
			"insert_key": createdKey,
		})
		return
	}

	if len(responseData.APIAccessCreateKeys.Errors) > 0 {
		http.Error(w, fmt.Sprintf("API returned an error: %v", responseData.APIAccessCreateKeys.Errors), http.StatusBadRequest)
		return
	}

	log.Println("No keys were created and no errors were returned by the API")
	http.Error(w, "No key was created", http.StatusInternalServerError)
}

// Delete an API key
func (s *Server) deleteApiKey(w http.ResponseWriter, r *http.Request) {
	log.Println("Received request to delete a key")

	apiKey := os.Getenv("NEW_RELIC_API_KEY")
	if apiKey == "" {
		http.Error(w, `{"error": "Missing NEW_RELIC_API_KEY"}`, http.StatusUnauthorized)
		return
	}

	var request DeleteKeyRequest
	err := json.NewDecoder(r.Body).Decode(&request)
	if err != nil || request.ID == "" {
		log.Printf("Invalid request: missing or invalid key ID. Status Code: %d", http.StatusBadRequest)
		return
	}

	mutation := fmt.Sprintf(`
	mutation {
		apiAccessDeleteKeys(keys: { ingestKeyIds: ["%q"] }) {
			deletedKeys {
				id
			}
			errors {
				message
			}
		}
	}`, request.ID)

	req := graphql.NewRequest(mutation)

	req.Header.Set("API-Key", apiKey)
	req.Header.Set("Content-Type", "application/json")

	var responseData DeleteKeysResponse

	ctx := context.Background()

	err = s.client.Run(ctx, req, &responseData)
	if err != nil {
		log.Printf("Error executing GraphQL request: %v", err)
		http.Error(w, fmt.Sprintf(`{"error": "Failed to delete key", "details": "%s"}`, err.Error()), http.StatusInternalServerError)
		return
	}

	if len(responseData.ApiAccessDeleteKeys.Errors) > 0 {
		errorMessages := []string{}
		for _, e := range responseData.ApiAccessDeleteKeys.Errors {
			errorMessages = append(errorMessages, e.Message)
		}
		log.Printf("Failed to delete key: %v, Status Code: %d", errorMessages, http.StatusInternalServerError)
		return
	}

	log.Printf("Successfully deleted key: Status Code=%d", http.StatusOK)
	w.WriteHeader(http.StatusOK)
	json.NewEncoder(w).Encode(map[string]any{
		"deleted_key": request.ID,
	})
}

func GetClient() (*graphql.Client, error) {
	newRelicGraphQLEndpoint := "https://api.eu.newrelic.com/graphql"
	client := graphql.NewClient(newRelicGraphQLEndpoint)
	log.Println("Successfully connected to NerdGraph client")
	return client, nil
}

func main() {
	err := godotenv.Load()
	if err != nil {
		log.Fatal("Error loading .env file")
	}

	apiKey := os.Getenv("NEW_RELIC_API_KEY")
	if apiKey == "" {
		log.Fatalf("Missing NEW_RELIC_API_KEY.")
	}

	client, err := GetClient()
	if err != nil {
		log.Fatalf("Failed to initialize GraphQL client: %v", err)
	}

	server := &Server{
		client: client,
		apiKey: apiKey,
	}

	r := mux.NewRouter()
	r.HandleFunc("/create-insert-key", server.createApiKey).Methods("POST")
	r.HandleFunc("/delete-key", server.deleteApiKey).Methods("DELETE")

	port := ":8080"
	fmt.Println("Server is running on port", port)
	log.Fatal(http.ListenAndServe(port, r))
}
