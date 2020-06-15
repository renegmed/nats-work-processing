package main

import (
	"encoding/json"
	"fmt"
	"log"
	"math/rand"
	"os"
	"runtime"
	"time"

	nats "github.com/nats-io/nats.go"
)

type Task struct {
	ID   string `json:"ID"`
	Name string `json:"name"`
}

type Station struct {
	ID      string `json:"ID"`
	Name    string `json:"name"`
	Jobtype string `json:"jobtype"`
}

type stations struct {
	stations []Station
}

func newStations() stations {
	s := []Station{
		Station{"101", "Station 101", "machine shop"},
		Station{"201", "Station 201", "machine shop"},
		Station{"301", "Station 301", "machine shop"},
	}
	return stations{s}
}

// assingWork goal if to find the first station that is available
// to do the work
func (s *stations) assignWork(conn *Connector, task Task) {
	rand.Seed(time.Now().UnixNano())
	start := rand.Intn(len(s.stations))
	lastIndx := len(s.stations) - 1
	i := start

	data, err := json.Marshal(task)
	if err != nil {
		log.Println("Error on marshalling task,", err)
		return
	}
	for {

		log.Println("Request to ", "Station.Need.Work."+s.stations[i].ID)

		resp, err := conn.nc.Request("Station.Need.Work."+s.stations[i].ID, data, 500*time.Millisecond)
		if err == nil {
			log.Printf("Received confirmation request Station.Need.Work task \n\t%v\n\t Message: %v", task, string(resp.Data))

			if string(resp.Data) == "accepted" {
				break
			}
		}

		if err != nil {
			log.Printf("Error on request Station.Need.Work task \n\t%v, \n\t%v ", task, err)
		}

		i++

		if i == lastIndx+1 {
			i = 0
		}

		if i == start {
			// done, went through all stations
			break
		}
	}
}

func execute() error {
	clientID := os.Getenv("CLIENT_ID")
	natsServers := os.Getenv("NATS_SERVER_ADDR")

	connector := NewConnector(clientID)

	log.Printf("clientID: %s, natsServers: %s\n", clientID, natsServers)

	err := connector.SetupConnectionToNATS(natsServers, nats.MaxReconnects(-1))
	if err != nil {
		return fmt.Errorf("Problem setting up connection to NATS servers, %v", err)
	}
	defer connector.Shutdown()

	go subscribe(connector)

	select {}

	return nil
}

func subscribe(conn *Connector) {

	conn.nc.Subscribe("Work.Available", func(m *nats.Msg) {

		log.Println("Receive Work.Available message", string(m.Data))

		stations := newStations()

		var tasks []Task
		err := json.Unmarshal(m.Data, &tasks)
		if err != nil {
			log.Println("Error on unmarshalling task list", err)
			return
		}

		for _, task := range tasks {
			stations.assignWork(conn, task)
			// _, err := assignToStationNewTask(conn, task)
			// if err != nil {
			// 	log.Println(err)
			// }
		}
	})

}

func assignToStationNewTask(conn *Connector, task Task) (bool, error) {

	log.Println("Finding station to assign task ID", task.ID)

	data, err := json.Marshal(task)
	if err != nil {
		return false, fmt.Errorf("Error on marshalling task", err)
	}
	err = conn.nc.Publish("Station.Need.Work", data)
	if err != nil {
		return false, fmt.Errorf("Error on request Station.Need.Work -", err)
	}

	log.Println("Work stations has been notified for available works.")

	return true, nil
}

func main() {
	err := execute()
	if err != nil {
		log.Println(err)
	}
	runtime.Goexit()
}
