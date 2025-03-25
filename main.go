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


func createApiKey(w http.ResponseWriter, r *http.Request) {
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

	// Create GraphQL request
	client := graphql.NewClient(newRelicGraphQLEndpoint)
	req := graphql.NewRequest(mutation)

	// Set headers
	req.Header.Set("API-Key",apiKey )
	req.Header.Set("Content-Type", "application/json")

	// Execute GraphQL request
	ctx := context.Background()
	var responseData NewRelicResponse
	if err := client.Run(ctx, req, &responseData); err != nil {
		http.Error(w, `{"error": "Failed to create insert key", "details": "`+err.Error()+`"}`, http.StatusInternalServerError)
		log.Println(err)
		return
	}

	// Return the created key
	if len(responseData.APIAccessCreateKeys.CreatedKeys) > 0 {
		w.WriteHeader(http.StatusOK)
		json.NewEncoder(w).Encode(map[string]any{
			"insert_key": responseData.APIAccessCreateKeys.CreatedKeys[0],
			
		})
	} else {
		http.Error(w, `{"error": "No key was created"}`, http.StatusInternalServerError)
		return
	}

		// Check for errors in response
		if len(responseData.APIAccessCreateKeys.Errors) > 0 {
			w.WriteHeader(http.StatusBadRequest)
			json.NewEncoder(w).Encode(map[string]any{
				"error":  "API returned an error",
				"details": responseData.APIAccessCreateKeys.Errors,
			})
	
			return
		}	
}

func main() {
	err := godotenv.Load()
	
	if err != nil {
		log.Fatal("Error loading .env file")
	  }
	//   client := graphql.NewClient(newRelicGraphQLEndpoint)
	//   if err != nil {
	// 	println("there is an error", client)
	//   }
  
	r := mux.NewRouter()
	r.HandleFunc("/create-insert-key", createApiKey).Methods("POST")

	port := ":8080"
	fmt.Println("Server is running on port", port)
	log.Fatal(http.ListenAndServe(port, r))
}
