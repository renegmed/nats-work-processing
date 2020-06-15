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
	ID   string `json:"ID"`
	Name string `json:"name"`
	Mark bool
}

type TaskResponse struct {
	ResponseID string `json:"response_id"`
	RequestID  string `json:"request_id"`
	Name       string `json:"name"`
	Notes      string `jsone:"notes"`
}

type Tasks struct {
	mutex sync.Mutex
	tasks []Task
}

func execute(tasks *Tasks) error {
	clientID := os.Getenv("CLIENT_ID")
	natsServers := os.Getenv("NATS_SERVER_ADDR")

	connector := NewConnector(clientID)

	log.Printf("clientID: %s, natsServers: %s\n", clientID, natsServers)

	err := connector.SetupConnectionToNATS(natsServers, nats.MaxReconnects(-1))
	if err != nil {
		return fmt.Errorf("Problem setting up connection to NATS servers, %v", err)
	}
	defer connector.Shutdown()

	go subscribeToTasks(connector, tasks)

	select {}

	return nil
}

func subscribeToTasks(conn *Connector, tasks *Tasks) {

	log.Println("i am in @ qhandler subcribe Add.Task.To.Queue")

	conn.nc.Subscribe("Add.Task.To.Queue", func(m *nats.Msg) {
		log.Println("Subscribing to Add.Task.To.Queue")
		var task Task
		err := json.Unmarshal(m.Data, &task)
		if err != nil {
			log.Println("Error while unmarshaling Add.Task.To.Queue request")
		}

		tasks.load(task)

		resp := TaskResponse{"200121", task.ID, task.Name, "Notes from qhandler..."}
		data, err := json.Marshal(resp)
		if err == nil {
			log.Println("Responding to Add.Task.To.Queue\n\t", resp)
			m.Respond(data)
		} else {
			log.Println("Error while marshalling Add.Task.To.Queue response", err)
		}
	})

	conn.nc.Subscribe("All.NotStarted.Tasks", func(m *nats.Msg) {
		data, _ := json.Marshal(tasks.list())
		m.Respond(data)
	})

	conn.nc.Subscribe("Publish.Available.Work", func(m *nats.Msg) {
		log.Println("Receive message Publish.Available.Work")
		tks := []Task{}
		tasks.mutex.Lock()
		for _, t := range tasks.tasks {
			if !t.Mark { // not assigned
				tks = append(tks, t)
			}
		}
		tasks.mutex.Unlock()

		publish(conn, tks)
	})

	conn.nc.Subscribe("Util.Clear.Available.Work", func(m *nats.Msg) {
		log.Println("Receive message Util.Clear.Available.Work")
		tasks.clear()

	})

	conn.nc.Subscribe("Notification.Assign.Work.Station", func(m *nats.Msg) {
		log.Println("Receive message Notification.Assign.Work.Station")
		type AssignedTask struct {
			ID           string    `json:"ID"`
			Name         string    `json:"name"`
			Status       string    `json:"status"`
			AssignedTo   string    `json:"assigned_to"`
			timeAssigned time.Time `json:"time_assigned"`
		}

		var task AssignedTask
		err := json.Unmarshal(m.Data, &task)
		if err != nil {
			log.Println("Error on unmarshalling received assigned task")
			return
		}

		log.Printf("Task marked, not available\n\t%v\n", task)

		tasks.mark(task.ID)

		log.Printf("Current list: \n\t%v", tasks.list())

	})

}

func publish(conn *Connector, tasks []Task) {

	log.Println("Requested to publish available works, ", tasks)
	data, err := json.Marshal(tasks)
	if err != nil {
		log.Println("Error when marshalling tasks", err)
		return
	}
	conn.nc.Publish("Work.Available", data)
}

// load NOT-STARTED task, mark = false
func (t *Tasks) load(task Task) {
	t.mutex.Lock()
	defer t.mutex.Unlock()
	t.tasks = append(t.tasks, task)
}

func (t *Tasks) next() (Task, bool) {
	t.mutex.Lock()
	defer t.mutex.Unlock()
	for _, t := range t.tasks {
		if !t.Mark {
			return t, true
		}
	}
	return Task{}, false
}

func (t *Tasks) mark(id string) {
	t.mutex.Lock()
	defer t.mutex.Unlock()
	for i, task := range t.tasks {
		log.Printf("+++ task.ID: %s  id: %v\n", task.ID, id)
		if task.ID == id {
			t.tasks[i].Mark = true
			return
		}
	}
	return
}

func (t *Tasks) list() []Task {
	return t.tasks
}

func (t *Tasks) clear() {
	t.mutex.Lock()
	defer t.mutex.Unlock()
	t.tasks = []Task{}
}

func main() {
	tasks := Tasks{}

	err := execute(&tasks)
	if err != nil {
		log.Println(err)
	}
	runtime.Goexit()
}
