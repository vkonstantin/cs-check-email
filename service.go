package main

import (
	"encoding/json"
	"fmt"
	"io/ioutil"
	"log"
	"net/http"
	"os"
	"context"
	"cloud.google.com/go/pubsub"
)

func publish(ctx context.Context, topic string, msg map[string]interface{}) error {
	client, err := pubsub.NewClient(ctx, os.Getenv("GOOGLE_CLOUD_PROJECT"))
	if err != nil {
		log.Printf("pubsub.NewClient: %v", err)
		return nil
	}

	t := client.Topic(topic)
	payload, _ := json.Marshal(msg)
	id, err := t.Publish(ctx, &pubsub.Message{ Data: payload }).Get(ctx)
  if err != nil {
    log.Printf("pubsub.Get: %v", err)
		return nil
  }
	log.Printf("Published a message; msg ID: %v\n", id)
	return nil
}

func main() {
	if os.Getenv("NEVERBOUNCE_API_KEY") == "" {
		log.Printf("NEVERBOUNCE_API_KEY environment variable must be set.")
		os.Exit(1)
	}
	if os.Getenv("NEVERBOUNCE_API_URL") == "" {
		log.Printf("NEVERBOUNCE_API_URL environment variable must be set.")
		os.Exit(1)
	}
	if os.Getenv("PUBSUB_TOPIC_ERROR") == "" {
		log.Printf("PUBSUB_TOPIC_ERROR environment variable must be set.")
		os.Exit(1)
	}
	if os.Getenv("PUBSUB_TOPIC_NEXT") == "" {
		log.Printf("PUBSUB_TOPIC_NEXT environment variable must be set.")
		os.Exit(1)
	}
	proj := os.Getenv("GOOGLE_CLOUD_PROJECT")
	if proj == "" {
		log.Printf("GOOGLE_CLOUD_PROJECT environment variable must be set.")
		os.Exit(1)
	}
	http.HandleFunc("/", handler)
	port := os.Getenv("PORT")
	if port == "" {
		port = "8080"
		log.Printf("Defaulting to port %s", port)
	}
	log.Printf("Listening on port %s", port)
	if err := http.ListenAndServe(":"+port, nil); err != nil {
		log.Fatal(err)
	}
}

type PubSubMessage struct {
	Message struct {
		Data []byte `json:"data,omitempty"`
		ID   string `json:"id"`
	} `json:"message"`
	Subscription string `json:"subscription"`
}

type Payload struct {
	Uid       string `json:"uid"`
	Email     string `json:"email"`
	FirstName	string `json:"first_name"`
	LastName	string `json:"last_name"`
}

func handler(w http.ResponseWriter, r *http.Request) {
	var m PubSubMessage
	body, err := ioutil.ReadAll(r.Body)
	if err != nil {
		log.Printf("iotuil.ReadAll: %v", err)
		http.Error(w, "Bad Request", http.StatusBadRequest)
		return
	}
	if err := json.Unmarshal(body, &m); err != nil {
		log.Printf("json.Unmarshal: %v", err)
		http.Error(w, "Bad Request", http.StatusBadRequest)
		return
	}
	var p Payload
	if err := json.Unmarshal(m.Message.Data, &p); err != nil {
		log.Printf("json.Unmarshal: %v", err)
		http.Error(w, "Bad Request", http.StatusBadRequest)
		return
	}
	log.Printf("params: %s", string(m.Message.Data))

	api_key := os.Getenv("NEVERBOUNCE_API_KEY")
	api_url := os.Getenv("NEVERBOUNCE_API_URL")
	url := fmt.Sprintf("%s?key=%s&email=%s", api_url, api_key, p.Email)
	//log.Printf("url: %s", url)
	resp, err := http.Get(url)
	if err != nil {
		error_topic := os.Getenv("PUBSUB_TOPIC_ERROR")
		log.Printf("error: %v %v", err, resp)
		publish(context.Background(), error_topic, map[string]interface{}{
			"uid":    		p.Uid,
			"email":			p.Email,
			"error_type": "request_error",
			"error": 			fmt.Sprintf("%v", err),
			"response": 	fmt.Sprintf("%v", resp),
		})
		return
	} else {
		var dat map[string]interface{}
		b, _ := ioutil.ReadAll(resp.Body)
		if err := json.Unmarshal(b, &dat); err != nil {
        log.Printf("error parsing resp: %v %v", resp, err)
    }
		log.Printf("parsed resp: %v", dat)
		result := dat["result"].(string)
		status := dat["status"].(string)
		if (status == "success" && result == "valid") {
			next_topic := os.Getenv("PUBSUB_TOPIC_NEXT")
			publish(context.Background(), next_topic, map[string]interface{}{
				"msg": map[string]interface{}{
					"uid":		    p.Uid,
					"email":			p.Email,
					"first_name":	p.FirstName,
					"last_name":	p.LastName,
				},
				"typ": "create_lead",
			})
		} else {
			error_topic := os.Getenv("PUBSUB_TOPIC_ERROR")
			publish(context.Background(), error_topic, map[string]interface{}{
				"uid":    		p.Uid,
				"email":			p.Email,
				"error_type": "response_error",
				"response": 	string(b),
			})
		}
		return
	}
	defer resp.Body.Close()
}
