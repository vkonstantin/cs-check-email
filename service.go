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
	"cloud.google.com/go/datastore"
	"wgames.com/send-email/marketo"
)

var (
	error_topic		 string
	proj        	 string
	client      	 marketo.Client
	marketo_id  	 string
	marketo_secret string
	marketo_url		 string
)

type PubSubMessage struct {
	Message struct {
		Data []byte `json:"data,omitempty"`
		ID   string `json:"id"`
	} `json:"message"`
	Subscription string `json:"subscription"`
}

type Payload struct {
	Leads      []Lead `json:"leads"`
	Tokens     []Token `json:"tokens"`
}

type Lead struct {
	Uid string `json:"uid"`
	Id int `json:"id,omitempty"`
}

type Token struct {
	Name string `json:"name"`
	Value string `json:"value"`
}

type MarketoReq struct {
	Input Payload `json:"input"`
}

type EmailContact struct {
	LeadId int
	Uid string
	Email string
	IsSubscribed bool
	K *datastore.Key `datastore:"__key__"`
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
	if error_topic = os.Getenv("PUBSUB_TOPIC_ERROR"); error_topic == "" {
		log.Printf("PUBSUB_TOPIC_ERROR environment variable must be set.")
		os.Exit(1)
	}
	if proj = os.Getenv("GOOGLE_CLOUD_PROJECT"); proj == "" {
		log.Printf("GOOGLE_CLOUD_PROJECT environment variable must be set.")
		os.Exit(1)
	}
	if marketo_id = os.Getenv("MARKETO_ID"); marketo_id == "" {
		log.Printf("MARKETO_ID environment variable must be set.")
		os.Exit(1)
	}
	if marketo_secret = os.Getenv("MARKETO_SECRET"); marketo_secret == "" {
		log.Printf("MARKETO_SECRET environment variable must be set.")
		os.Exit(1)
	}
	if marketo_url = os.Getenv("MARKETO_URL"); marketo_url == "" {
		log.Printf("MARKETO_URL environment variable must be set.")
		os.Exit(1)
	}
	config := marketo.ClientConfig{
    ID:       marketo_id,
    Secret:   marketo_secret,
    Endpoint: marketo_url,
    Debug:    true,
	}
	var err error
	client, err = marketo.NewClient(config)
	if err != nil {
	    log.Printf("Can't connect to marketo: %v", err)
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
	log.Printf("body: %s", string(body))
	if err != nil {
		return fmt.Errorf("iotuil.ReadAll: %v", err)
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

	ctx := context.Background()
	dsClient, err := datastore.NewClient(ctx, proj)

	for _, lead := range p.Leads {
		log.Printf("uid %s", lead.Uid)
		k := datastore.NameKey("EmailContact", lead.Uid, nil)
		e := new(EmailContact)
		if err := dsClient.Get(ctx, k, e); err != nil {
			log.Printf("can't get record for %s", lead.Uid)
		} else {
			lead.Id = e.LeadId
			log.Printf("uid %s %d", lead.Uid, lead.Id)
		}
	}

	req := new(MarketoReq)
	req.Input = p
	dataInBytes, err := json.Marshal(req)
	log.Printf("req: %s", string(dataInBytes))
	response, err := client.Post("/rest/v1/campaigns/1422/trigger.json", dataInBytes)
	if err != nil {
	    log.Printf("error posting to marketo %v", err)
	}
	if !response.Success {
	    log.Printf("response is not success %v", response.Errors)
	}
	log.Printf("Campaign triggered")
	return nil
}
