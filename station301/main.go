package main

import (
	"encoding/json"
	"fmt"
	"log"
	"os"
	"runtime"
	"sync"
	"time"

	nats "github.com/nats-io/nats.go"
)

type Task struct {
	ID           string    `json:"ID"`
	Name         string    `json:"name"`
	Status       string    `json:"status"`
	AssignedTo   string    `json:"assigned_to"`
	TimeAssigned time.Time `json:"time_assigned"`
	TimeFinished time.Time `json:"time_finished"`
}

type Station struct {
	ID      string `json:"ID"`
	Name    string `json:"name"`
	Jobtype string `json:"jobtype"`
	Status  string `json:"status"`
	Mutex   sync.Mutex
	Task    Task
}

var station Station

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
	go utils(connector)

	select {}

	return nil
}

func subscribe(conn *Connector) {

	log.Println("i am in @station301 subcriber")

	conn.nc.Subscribe("Station.Need.Work."+station.ID, func(m *nats.Msg) {

		log.Printf("Work notice %s: \n\t%v\n ", "Station.Need.Work."+station.ID, string(m.Data))

		var task Task
		err := json.Unmarshal(m.Data, &task)
		if err != nil {
			log.Println("Error while unmarshaling Need.Work.Station subscription")
			return
		}

		if station.Status == "available" {
			task.Status = "proposed"
			task.AssignedTo = station.ID
			task.TimeAssigned = time.Now()
			ok, err := proposeAssignToWork(conn, task)
			if err != nil {
				log.Println("Error on proposal Assign to Work", err)
				return
			}
			if ok {
				m.Respond([]byte("accepted"))
			} else {
				m.Respond([]byte("rejected"))
			}

		} else {
			log.Println("This station could not be assigned to a new job at this moment. Status:", station.Status)
			m.Respond([]byte("rejected"))
		}
	})

	conn.nc.Subscribe("Approved.Assign.Work."+station.ID, func(m *nats.Msg) {

		log.Println("Received work assignment approval, station status", station.Status)

		if station.Status == "available" {

			var task Task
			err := json.Unmarshal(m.Data, &task)
			if err != nil {
				log.Println("Error while unmarshalingApproved.Assign.Work.Station subscription")
				return
			}
			station.Mutex.Lock()
			station.Task = task
			station.Status = "working"
			station.Mutex.Unlock()
			m.Respond(nil)

			log.Println("Work station 101 current status\n\t", station)
		} else {
			log.Println("This station is not available for new work. Not responding for new assignment")
		}

	})
}

func proposeAssignToWork(conn *Connector, task Task) (bool, error) {

	log.Println("Propose work assignment task ID", task.ID)

	data, err := json.Marshal(task)
	if err != nil {
		return false, fmt.Errorf("Error on marshalling task", err)
	}
	err = conn.nc.Publish("Proposed.Assign.Work", data)
	if err != nil {
		return false, fmt.Errorf("Error on request Proposed.Assign.Work -", err)
	}

	log.Println("Proposed.Assign.Work has been published.")

	return true, nil
}

func utils(conn *Connector) {

	conn.nc.Subscribe("Util.About.Station."+station.ID, func(m *nats.Msg) {
		log.Println("Current station status", station.Status)
		data, err := json.Marshal(station)
		if err != nil {
			log.Println("Error on marshalling station,", err)
			return
		}
		m.Respond(data)
	})

	conn.nc.Subscribe("Util.Finish.Work."+station.ID, func(m *nats.Msg) {

		log.Println("Finishing the work, then report to dispatcher ")
		if station.Task.ID == "" {
			log.Println("Station has no current work.")
			return
		}
		station.Mutex.Lock()
		station.Task.TimeFinished = time.Now()
		station.Mutex.Unlock()

		ok, err := publishFinishWork(conn, station.Task)
		if err != nil {
			log.Printf("Error on publishing finish work,\n\t%v\n", station.Task)
			return
		}
		if ok {
			station.Mutex.Lock()
			station.Status = "available"
			station.Task = Task{}
			station.Mutex.Unlock()
		}

		log.Println("Current station status", station.Status)

	})
}

func publishFinishWork(conn *Connector, task Task) (bool, error) {

	log.Println("Reporting finish work to Dispatcher, receive receipt confirmation")

	data, err := json.Marshal(task)
	if err != nil {
		return false, fmt.Errorf("Error on marshalling task, %v ", err)
	}
	resp, err := conn.nc.Request("Finish.Work", data, 500*time.Millisecond)
	if err != nil {
		return false, fmt.Errorf("Error on request Finish.Worktask \n\t%v, \n\t%v ", task, err)
	}

	if resp == nil {
		return false, fmt.Errorf("Problem, has response toFinish.Work but no message on task \n\t%v\n", task)
	}

	log.Printf("Received confirmation request Finish.Work task \n\t%v\n\t Message: %v", task, string(resp.Data))

	return true, nil

}

func main() {
	station = Station{}
	station.ID = "301"
	station.Name = "Work station 301"
	station.Jobtype = "lathe"
	station.Status = "available"

	err := execute()
	if err != nil {
		log.Println(err)
	}
	runtime.Goexit()
}
