package main

import (
	"encoding/json"
	"fmt"
	"log"
	"net/http"
	"os"
	"runtime"
	"time"

	nats "github.com/nats-io/nats.go"
	"github.com/satori/uuid"
)

type Task struct {
	ID   string `json:"ID"`
	Name string `json:"name"`
}

type qTask struct {
	ID   string `json:"ID"`
	Name string `json:"name"`
	Mark bool   `json:"assigned"`
}

type server struct {
	nc *nats.Conn
}

var natsServer server

func testHandler(w http.ResponseWriter, r *http.Request) {
	switch r.Method {
	case http.MethodPost:

		log.Println("Clear databases")

		err := clearDb()
		if err != nil {

		}

		newUUID := uuid.NewV4()
		taskId := newUUID.String()

		log.Println("Submitting tasks queue handler")

		tasks := []Task{
			{taskId, "Bore 5mm hole"},
			//{"10002", "Install 3/4 screw"}
		}

		err = addToTaskQueue(tasks)
		if err != nil {
			responseError(w, http.StatusExpectationFailed, fmt.Sprintf("%v", err))
			return
		}

		log.Println("Verify Queue handler receives the tasks")
		err = verifyQHandlerReceiveTasks(taskId)
		if err != nil {
			responseError(w, http.StatusExpectationFailed, fmt.Sprintf("%v", err))
			return
		}

		log.Println("Queue handler announce to Router new works available")

		time.Sleep(1 * time.Second)
		err = publishWorkAvailable()
		if err != nil {
			responseError(w, http.StatusExpectationFailed, fmt.Sprintf("%v", err))
			return
		}

		log.Println("Router tries to find stations available to accept the new works")

		log.Println("Verify only one station received the work. Get which station received the work. Verify the status")

		time.Sleep(1 * time.Second)
		assignedStationId, err := verifyOnlyOneStationAcceptedWork(taskId)
		if err != nil {
			responseError(w, http.StatusExpectationFailed, fmt.Sprintf("Failed on station acceptance of work, %v", err))
			return
		}

		log.Println("AssignedStationID:", assignedStationId)

		log.Println("Check Dispatcher what Station work is assigned to ")

		time.Sleep(1 * time.Second)
		err = verifyDispatcherStationWorkAssignedTo(taskId, assignedStationId)
		if err != nil {
			responseError(w, http.StatusExpectationFailed, fmt.Sprintf("Failed, dispatcher record on station assigned work, %v", err))
			return
		}

		log.Println("Verify Queue handler received assigned work and 'mark' field is changed to true.")
		err = verifyQhandlerMarkedRecord(taskId)
		if err != nil {
			responseError(w, http.StatusExpectationFailed, fmt.Sprintf("Failed qHandler to mark task assigned, %v", err))
			return
		}

		log.Println("Make assigned Station finish the work")
		time.Sleep(1 * time.Second)
		err = finishWork(assignedStationId)
		if err != nil {
			responseError(w, http.StatusExpectationFailed, fmt.Sprintf("Failed station to finish the work, %v", err))
			return
		}

		log.Println("Verify Dispatcher changed the work status from 'assigned' to 'finished' ")
		time.Sleep(1 * time.Second)
		err = verifyDispatcherChangedStatus(assignedStationId, taskId)
		if err != nil {
			responseError(w, http.StatusExpectationFailed, fmt.Sprintf("Failed dispatcher to change status from assigned to finished, %v", err))
			return
		}

		log.Println("Verify the Station is available for a new work.")
		time.Sleep(1 * time.Second)
		err = verifyStationAvailableForNewWork(assignedStationId)
		if err != nil {
			responseError(w, http.StatusExpectationFailed, fmt.Sprintf("Failed station to change status to 'available', %v", err))
			return
		}

		type response struct {
			Message string `json:"message"`
		}

		responseOk(w, response{Message: "Test has been successful."})
	}
}

func clearDb() error {

	err := natsServer.nc.Publish("Util.Clear.Available.Work", nil)
	if err != nil {
		return fmt.Errorf("Problem, publishing Util.Clear.Available.Work, %v", err)
	}

	natsServer.nc.Publish("Util.Dispatcher.Clear", nil)

	type Station struct {
		ID   string `json:"ID"`
		Name string `json:"name"`
	}

	stations := []Station{
		Station{"101", "Station 101"},
		Station{"201", "Station 201"},
		Station{"301", "Station 301"},
	}

	for _, station := range stations {
		natsServer.nc.Publish(fmt.Sprintf("Util.Station.Clear.%s", station.ID), nil)
	}

	return nil
}

func addToTaskQueue(tasks []Task) error {

	for _, task := range tasks {
		data, err := json.Marshal(task)
		if err != nil {
			return fmt.Errorf("Error on marshalling tasks.")
		}
		resp, err := natsServer.nc.Request("Add.Task.To.Queue", data, 1000*time.Millisecond)
		if err != nil {
			return fmt.Errorf("Error on request 'Add.Task.To.Queue")
		}

		type TaskResponse struct {
			ResponseID string `json:"response_id"`
			RequestID  string `json:"request_id"`
			Name       string `json:"name"`
			Notes      string `jsone:"notes"`
		}

		var respData TaskResponse
		err = json.Unmarshal(resp.Data, &respData)
		if err != nil {
			return fmt.Errorf("Error on unmarshalling response data")
		}

		log.Println("Response from QHandler: ", respData)

	}

	return nil
}

func publishWorkAvailable() error {

	err := natsServer.nc.Publish("Publish.Available.Work", nil)
	if err != nil {
		return fmt.Errorf(fmt.Sprintf("Problem publish available works.", err))
	}
	return nil
}

func verifyQHandlerReceiveTasks(taskID string) error {

	tasks, err := qHandlerNotStartedTaskList()
	if err != nil {
		return err
	}

	if len(tasks) == 0 {
		return fmt.Errorf("Error, task list is empty.")
	}

	log.Println(tasks)

	for _, task := range tasks {
		if task.ID == taskID {
			return nil
		}
	}
	return fmt.Errorf("Qhandler task %v has not been received,", taskID)
}
func qHandlerNotStartedTaskList() ([]qTask, error) {

	resp, err := natsServer.nc.Request("All.NotStarted.Tasks", nil, 500*time.Millisecond)
	if err != nil {
		return []qTask{}, fmt.Errorf("Error on request 'All.NotStarted.Tasks' %v", err)
	}

	var tasks []qTask
	err = json.Unmarshal(resp.Data, &tasks)
	if err != nil {
		return []qTask{}, fmt.Errorf("Error on unmarshal tasks %v", err)
	}
	return tasks, nil
}

func verifyOnlyOneStationAcceptedWork(taskId string) (string, error) {

	type Station struct {
		ID      string `json:"ID"`
		Name    string `json:"name"`
		Jobtype string `json:"jobtype"`
	}

	stations := []Station{
		Station{"101", "Station 101", "machine shop"},
		Station{"201", "Station 201", "machine shop"},
		Station{"301", "Station 301", "machine shop"},
	}

	var assignedStation = ""
	var findCounter = 0
	for _, station := range stations {
		resp, err := natsServer.nc.Request(fmt.Sprintf("Util.About.Station.%s", station.ID), nil, 500*time.Millisecond)
		if err != nil {
			return "", fmt.Errorf("Error on request 'Util.About.Station' %v", err)
		}

		log.Println(fmt.Sprintf("Response for station: %s task id: %s Util.About.Station \n\t %s", station.ID, taskId, string(resp.Data)))

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
			Task    Task   `json:"task"`
		}

		var station Station

		err = json.Unmarshal(resp.Data, &station)
		if err != nil {
			log.Println("Error on unmarshal station", err)
		}

		log.Println("+++ station Details\n\t", station)
		log.Println("+++ station.Task.ID\n\t", station.Task.ID)
		log.Println("+++ station.Status\n\t", station.Status)
		if station.Task.ID == taskId && station.Status == "working" {
			if findCounter == 1 {
				return "", fmt.Errorf("More than one station work is assigned.")
			}
			assignedStation = station.ID
			findCounter++
		}
	}
	if assignedStation != "" {
		return assignedStation, nil
	}
	return "", fmt.Errorf("No station is assigned for the task %v", taskId)
}

func verifyDispatcherStationWorkAssignedTo(taskId, stationId string) error {

	resp, err := natsServer.nc.Request("Util.Dispatcher.Database", nil, 500*time.Millisecond)
	if err != nil {
		return fmt.Errorf("Error on request Util.Dispatcher.Database, %v", err)
	}

	type Task struct {
		ID           string    `json:"ID"`
		Name         string    `json:"name"`
		Status       string    `json:"status"`
		AssignedTo   string    `json:"assigned_to"`
		TimeAssigned time.Time `json:"time_assigned"`
		TimeFinished time.Time `json:"time_finished"`
	}

	var tasks []Task

	err = json.Unmarshal(resp.Data, &tasks)
	if err != nil {
		return fmt.Errorf("Error on unmarshal task list, %v", err)
	}

	log.Println("Response from Util.Dispatcher.Database, tasks:\n\t", tasks)

	for _, task := range tasks {
		if task.ID == taskId && task.AssignedTo == stationId && task.Status == "assigned" {
			return nil
		}
	}
	return fmt.Errorf("Error, work %s should have been assigned to %s.", taskId, stationId)
}

func qHandlerAllTaskList() ([]qTask, error) {

	resp, err := natsServer.nc.Request("All.Tasks", nil, 500*time.Millisecond)
	if err != nil {
		return []qTask{}, fmt.Errorf("Error on request 'All.Tasks' %v", err)
	}

	var tasks []qTask
	err = json.Unmarshal(resp.Data, &tasks)
	if err != nil {
		return []qTask{}, fmt.Errorf("Error on unmarshal tasks %v", err)
	}
	return tasks, nil
}

func verifyQhandlerMarkedRecord(taskId string) error {
	tasks, err := qHandlerAllTaskList()
	if err != nil {
		return err
	}
	if len(tasks) == 0 {
		return fmt.Errorf("Error, task list is empty.")
	}

	log.Println("verifyQhandlerMarkedRecord - tasks\n\t", tasks)

	for _, task := range tasks {
		if task.ID == taskId && task.Mark {
			return nil
		}
	}
	return fmt.Errorf("Error, task should have marked as assigned.")
}

func finishWork(stationId string) error {

	err := natsServer.nc.Publish(fmt.Sprintf("Util.Finish.Work.%s", stationId), nil)
	if err != nil {
		return fmt.Errorf("Problem, publishing Util.Clear.Available.Work,", err)
	}

	return nil
}

func verifyDispatcherChangedStatus(stationId, taskId string) error {

	resp, err := natsServer.nc.Request("Util.Dispatcher.Database", nil, 500*time.Millisecond)
	if err != nil {
		return fmt.Errorf("Error on request Util.Dispatcher.Database, %v", err)
	}

	type Task struct {
		ID           string    `json:"ID"`
		Name         string    `json:"name"`
		Status       string    `json:"status"`
		AssignedTo   string    `json:"assigned_to"`
		TimeAssigned time.Time `json:"time_assigned"`
		TimeFinished time.Time `json:"time_finished"`
	}

	var tasks []Task

	err = json.Unmarshal(resp.Data, &tasks)
	if err != nil {
		return fmt.Errorf("Error on unmarshal task list, %v", err)
	}

	log.Println("Response from Util.Dispatcher.Database, tasks:\n\t", tasks)

	for _, task := range tasks {
		if task.ID == taskId && task.AssignedTo == stationId && task.Status == "finished" {
			return nil
		}
	}
	return fmt.Errorf("Error, work %s status of station %s have been 'finished' status.", taskId, stationId)
}

func verifyStationAvailableForNewWork(stationId string) error {
	resp, err := natsServer.nc.Request(fmt.Sprintf("Util.About.Station.%s", stationId), nil, 500*time.Millisecond)
	if err != nil {
		return fmt.Errorf("Error on request Util.About.Station.%s %v", stationId, err)
	}

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
		Task    Task   `json:"task"`
	}

	var station Station

	err = json.Unmarshal(resp.Data, &station)
	if err != nil {
		log.Println("Error on unmarshal station", err)
		return fmt.Errorf("Error Util.About.Station.%s unmarshalling station %v", stationId, err)
	}
	log.Println(".....verifyStationAvailableForNewWork(): ")
	log.Println("+++ station Details\n\t", station)
	log.Println("+++ station.Task.ID\n\t", station.Task.ID)
	log.Println("+++ station.Status\n\t", station.Status)
	if station.ID == stationId && station.Status == "available" {
		return nil
	}

	return fmt.Errorf("Error Util.About.Station.%s station has not been updated. %v", stationId, err)
}

func main() {
	serverPort := os.Getenv("SERVER_PORT")
	clientID := os.Getenv("CLIENT_ID")
	natsServers := os.Getenv("NATS_SERVER_ADDR")

	connector := NewConnector(clientID)

	log.Printf("serverPort: %s, clientID: %s, natsServers: %s\n", serverPort, clientID, natsServers)

	err := connector.SetupConnectionToNATS(natsServers, nats.MaxReconnects(-1))
	if err != nil {
		log.Printf("Problem setting up connection to NATS servers, %v", err)
		runtime.Goexit()
	}

	defer connector.Shutdown()

	natsServer = server{nc: connector.NATS()}

	http.HandleFunc("/test", testHandler)

	log.Printf("====== Test/Monitor server listening on port %s...", serverPort)
	if err := http.ListenAndServe(":"+serverPort, nil); err != nil {
		log.Fatal(err)
	}
}

func responseError(w http.ResponseWriter, code int, message string) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(code)

	body := map[string]string{
		"error": message,
	}
	json.NewEncoder(w).Encode(body)
}

func responseOk(w http.ResponseWriter, body interface{}) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(http.StatusOK)

	json.NewEncoder(w).Encode(body)
}
