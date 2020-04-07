package main

import (
	"context"
	"encoding/json"
	"fmt"
	"io/ioutil"
	"log"
	"net/http"
	"os"

	"cloud.google.com/go/pubsub"
)

var (
	api_key     string
	api_url     string
	error_topic string
	next_topic  string
	proj        string
)

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
	FirstName string `json:"first_name"`
	LastName  string `json:"last_name"`
}

type NextTopicMsg struct {
	Msg Payload `json:"msg"`
	Typ string  `json:"typ"`
}

type ErrPayload struct {
	Uid       string `json:"uid,omitempty"`
	Email     string `json:"email,omitempty"`
	ErrorType string `json:"error_type,omitempty"`
	Error     string `json:"error,omitempty"`
	Response  string `json:"response,omitempty"`
}

type Dat struct {
	Result string `json:"result,omitempty"`
	Status string `json:"status,omitempty"`
}

func publish(ctx context.Context, topic string, msg interface{}) error {
	client, err := pubsub.NewClient(ctx, proj)
	if err != nil {
		return err
	}

	t := client.Topic(topic)
	payload, err := json.Marshal(msg)
	if err != nil {
		return err
	}

	id, err := t.Publish(ctx, &pubsub.Message{Data: payload}).Get(ctx)
	if err != nil {
		return err
	}
	log.Printf("Published a message; msg ID: %v\n", id)
	return nil
}

func publishLogError(ctx context.Context, topic string, msg interface{}) {
	if err := publish(ctx, topic, msg); err != nil {
		log.Printf("error: %s on publish(%s, %v)", err, topic, msg)
	}
}

func init() {
	if api_key = os.Getenv("NEVERBOUNCE_API_KEY"); api_key == "" {
		log.Printf("NEVERBOUNCE_API_KEY environment variable must be set.")
		os.Exit(1)
	}
	if api_url = os.Getenv("NEVERBOUNCE_API_URL"); api_url == "" {
		log.Printf("NEVERBOUNCE_API_URL environment variable must be set.")
		os.Exit(1)
	}
	if error_topic = os.Getenv("PUBSUB_TOPIC_ERROR"); error_topic == "" {
		log.Printf("PUBSUB_TOPIC_ERROR environment variable must be set.")
		os.Exit(1)
	}
	if next_topic = os.Getenv("PUBSUB_TOPIC_NEXT"); next_topic == "" {
		log.Printf("PUBSUB_TOPIC_NEXT environment variable must be set.")
		os.Exit(1)
	}
	if proj = os.Getenv("GOOGLE_CLOUD_PROJECT"); proj == "" {
		log.Printf("GOOGLE_CLOUD_PROJECT environment variable must be set.")
		os.Exit(1)
	}
}

func main() {
	http.HandleFunc("/", commonHandler(handler))
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

func commonHandler(h func(http.ResponseWriter, *http.Request) error) func(http.ResponseWriter, *http.Request) {
	return func(w http.ResponseWriter, r *http.Request) {
		err := h(w, r)
		if err != nil {
			log.Printf("error: %s", err)
			http.Error(w, "Bad Request", http.StatusBadRequest)
		}
	}
}

func handler(w http.ResponseWriter, r *http.Request) error {
	body, err := ioutil.ReadAll(r.Body)
	if err != nil {
		return fmt.Errorf("iotuil.ReadAll: %w", err)
	}

	var m PubSubMessage
	if err := json.Unmarshal(body, &m); err != nil {
		return fmt.Errorf("json.Unmarshal: %v", err)
	}

	var p Payload
	if err := json.Unmarshal(m.Message.Data, &p); err != nil {
		return fmt.Errorf("json.Unmarshal: %v", err)
	}
	log.Printf("params: %s", string(m.Message.Data))

	url := fmt.Sprintf("%s?key=%s&email=%s", api_url, api_key, p.Email)
	//log.Printf("url: %s", url)
	resp, err := http.Get(url)
	if err != nil {
		log.Printf("error: %v %v", err, resp)
		publishLogError(context.Background(), error_topic, &ErrPayload{
			Uid:       p.Uid,
			Email:     p.Email,
			ErrorType: "request_error",
			Error:     fmt.Sprintf("%v", err),
			Response:  fmt.Sprintf("%v", resp),
		})
		return nil
	}

	var dat Dat
	b, err := ioutil.ReadAll(resp.Body)
	defer resp.Body.Close()
	if err != nil {
		return err
	}

	if err := json.Unmarshal(b, &dat); err != nil {
		return fmt.Errorf("error parsing resp: %v %v", resp, err)
	}
	log.Printf("parsed resp: %v", dat)

	if dat.Status == "success" && dat.Result == "valid" {
		publishLogError(context.Background(), next_topic, &NextTopicMsg{
			Msg: Payload{
				Uid:       p.Uid,
				Email:     p.Email,
				FirstName: p.FirstName,
				LastName:  p.LastName,
			},
			Typ: "create_lead",
		})
	} else {
		publishLogError(context.Background(), error_topic, &ErrPayload{
			Uid:       p.Uid,
			Email:     p.Email,
			ErrorType: "response_error",
			Response:  string(b),
		})
	}

	return nil
}
