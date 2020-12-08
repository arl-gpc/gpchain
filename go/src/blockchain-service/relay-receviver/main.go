package main

import (
	"fmt"
	"os"
	"flag"
	mqtt "github.com/eclipse/paho.mqtt.golang"
)

func main() {
	topic := flag.String("topic", "", "")
	id := flag.String("id", "", "")
	flag.Parse()

	opts := mqtt.NewClientOptions()
	opts.AddBroker("tcp://localhost:1883")
	opts.SetClientID(*id)
	opts.SetCleanSession(false)

	receivingChannel := make(chan [2]string)

	opts.SetDefaultPublishHandler(func(client mqtt.Client, msg mqtt.Message) {
		receivingChannel <- [2]string{msg.Topic(), string(msg.Payload())}
	})

	client := mqtt.NewClient(opts)
	if token := client.Connect(); token.Wait() && token.Error() != nil {
		fmt.Printf("Couldn't Connect: %s\n", token.Error())
		return
	}
	defer client.Disconnect(uint(250))
	
	if token := client.Subscribe(*topic, byte(0), nil); token.Wait() && token.Error() != nil {
		fmt.Println(token.Error())
		os.Exit(1)
	}

	for true {
		fmt.Printf("Waiting...\n")
		incoming, isOpen := <-receivingChannel
		if !isOpen {
			fmt.Println("Channel Closed")
			return
		}
		fmt.Printf("RECEIVED TOPIC: %s MESSAGE: %s\n", incoming[0], incoming[1])
	}
}