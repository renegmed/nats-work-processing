package main

/*
	Keeps track of work status
	Approves the proposed work assignment
	Informs other parties of work status

*/
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
	Mutex        sync.Mutex
}

type dispatcher struct {
	Tasks map[string]Task
	Mutex sync.Mutex
}

func (d *dispatcher) add(task Task) {
	if d.Tasks == nil {
		d.Tasks = map[string]Task{}
	}

	d.Mutex.Lock()
	d.Tasks[task.ID] = task
	d.Mutex.Unlock()
}

func (d *dispatcher) clear() {
	d.Mutex.Lock()
	d.Tasks = map[string]Task{}
	d.Mutex.Unlock()
}

func (d *dispatcher) update(task Task) {
	if d.Tasks == nil {
		d.Tasks = map[string]Task{}
	}

	d.Mutex.Lock()
	t, ok := d.Tasks[task.ID]
	if ok {
		t.TimeFinished = task.TimeFinished
		t.Status = "finished"

		d.Tasks[task.ID] = t

	} else {
		d.Tasks[task.ID] = task
	}
	d.Mutex.Unlock()
}

func (d *dispatcher) status(id, status string) {
	d.Mutex.Lock()
	defer d.Mutex.Unlock()
	task, found := d.Tasks[id]
	if found {
		task.Status = status
	}
}

func (d *dispatcher) list() []Task {
	list := []Task{}
	d.Mutex.Lock()
	defer d.Mutex.Unlock()
	for _, v := range d.Tasks {
		list = append(list, v)
	}
	return list
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

	dispatcher := new(dispatcher)
	go subscribe(connector, dispatcher)

	select {}

	return nil
}

func subscribe(conn *Connector, dispatcher *dispatcher) {

	log.Println("i am in @dispatcher subcriber")

	conn.nc.Subscribe("Util.Dispatcher.Database", func(m *nats.Msg) {

		log.Println("Send list of tasks currently in the database. list:\n\t", dispatcher.list())
		taskList := dispatcher.list()
		data, err := json.Marshal(taskList)
		if err != nil {
			log.Println("Error while marshaling task list in Util.Dispatcher.Database")
			return
		}
		m.Respond(data)
	})

	conn.nc.Subscribe("Util.Dispatcher.Clear", func(m *nats.Msg) {
		log.Println("Clearing dispatcher db")
		dispatcher.clear()
	})

	conn.nc.Subscribe("Proposed.Assign.Work", func(m *nats.Msg) {

		log.Println("Subscribing to Proposed.Assign.Work")

		var task Task
		err := json.Unmarshal(m.Data, &task)
		if err != nil {
			log.Println("Error while unmarshaling Proposed.Assign.Workrequest")
			return
		}

		// Sequence of events
		// 1. approve proposed work assignment
		// 2. add task to local list
		// 3. notify work station of approved work assignment
		// 4. notify other parties for work assignment
		task.Mutex.Lock()
		task.Status = "assigned"
		task.TimeAssigned = time.Now()
		task.Mutex.Unlock()

		log.Println("Topic: Proposed.Assign.Work, task to add\n\t", task)

		dispatcher.add(task)
		ok, err := publishApprovalWorkAssignment(conn, task)
		if err != nil {
			log.Println("Error on approving proposed work assignment", err)
			return
		}
		if ok {
			dispatcher.add(task)
			notifyWorkAssignment(conn, task)
		}
	})

	conn.nc.Subscribe("Finish.Work", func(m *nats.Msg) {

		log.Println("Rceiving finish work, sending signal as confirmation.")
		var task Task
		err := json.Unmarshal(m.Data, &task)
		if err != nil {
			log.Println("Error while unmarshaling Finish.Work")
			return
		}

		log.Printf("Finish work reported: \n\t%v\n", task)

		dispatcher.update(task)

		m.Respond(nil)

	})

}

func publishApprovalWorkAssignment(conn *Connector, task Task) (bool, error) {

	data, err := json.Marshal(task)
	if err != nil {
		return false, fmt.Errorf("Error on marshalling task, %v ", err)
	}
	resp, err := conn.nc.Request("Approved.Assign.Work."+task.AssignedTo, data, 1000*time.Millisecond)
	if err != nil {
		return false, fmt.Errorf("Error on request Approved.Assign.Work task \n\t%v, \n\t%v ", task, err)
	}

	if resp == nil {
		return false, fmt.Errorf("Problem, has response to Approved.Assign.Work but no message on task \n\t%v\n", task)
	}

	log.Printf("Received confirmation request Approved.Assign.Work task \n\t%v\n\t Message: %v", task, string(resp.Data))

	return true, nil
}

func notifyWorkAssignment(conn *Connector, task Task) (bool, error) {

	log.Printf("Notification on work assignment - \n\t%v\n", task)

	data, err := json.Marshal(task)
	if err != nil {
		return false, fmt.Errorf("Error work assignment notification marshalling", err)
	}
	conn.nc.Publish("Notification.Assign.Work.Station", data)
	return true, nil
}

func main() {

	err := execute()
	if err != nil {
		log.Println(err)
	}
	runtime.Goexit()
}
