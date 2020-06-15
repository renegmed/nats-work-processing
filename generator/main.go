package main

import (
	"encoding/json"
	"fmt"
	"io/ioutil"
	"log"
	"net/http"
	"os"
	"runtime"
	"strings"
	"time"

	nats "github.com/nats-io/nats.go"
)

type Task struct {
	ID   string `json:"ID"`
	Name string `json:"name"`
}

type server struct {
	nc *nats.Conn
}

var natsServer server

// Request to add new task to queue
func taskHandler(w http.ResponseWriter, r *http.Request) {
	switch r.Method {
	case http.MethodPost:

		log.Println("Requesting to add task to queue")

		body, _ := ioutil.ReadAll(r.Body)

		var task Task
		err := json.Unmarshal(body, &task)
		if err != nil {
			responseError(w, http.StatusBadRequest, "Invalid data")
			return
		}

		resp, err := natsServer.nc.Request("Add.Task.To.Queue", body, 500*time.Millisecond)
		if err != nil {
			responseError(w, http.StatusBadRequest, "Error on request 'Add.Task.To.Queue")
			return
		}

		if resp == nil {
			responseError(w, http.StatusBadRequest, "Problem, has response but no message.")
			return
		}

		// TODO:  handle confirmation to request
		log.Println("Response to request Add.Task.To.Queue", string(resp.Data))

		type response struct {
			ID      string `json:"id"`
			Message string `json:"message"`
		}

		responseOk(w, response{ID: task.ID, Message: "task added."})
	}
}

func taskListHandler(w http.ResponseWriter, r *http.Request) {
	switch r.Method {
	case http.MethodGet:

		resp, err := natsServer.nc.Request("All.NotStarted.Tasks", nil, 500*time.Millisecond)
		if err != nil {
			responseError(w, http.StatusBadRequest, "Error on request 'All.NotStarted.Tasks'")
			return
		}

		if resp == nil {
			responseError(w, http.StatusBadRequest, "Problem, has response but no message.")
			return
		}

		//log.Println("Response to request All.NotStarted.Tasks", string(resp.Data))

		type Task struct {
			ID   string `json:"ID"`
			Name string `json:"name"`
			Mark bool   `json:"assigned"`
		}

		var tasks []Task
		err = json.Unmarshal(resp.Data, &tasks)
		if err != nil {
			log.Println("Error on unmarshal tasks", err)
		}

		log.Println(tasks)
		responseOk(w, tasks)
	}
}

func clearTasksHandler(w http.ResponseWriter, r *http.Request) {
	switch r.Method {
	case http.MethodPost:
		err := natsServer.nc.Publish("Util.Clear.Available.Work", nil)
		if err != nil {
			responseError(w, http.StatusBadRequest, fmt.Sprintf("Problem, publishing Util.Clear.Available.Work,", err))
			return
		}
		type response struct {
			Message string `json:"message"`
		}
		responseOk(w, response{Message: "Available works list is cleared."})
	}
}

func stationHandler(w http.ResponseWriter, r *http.Request) {
	urlPathSegments := strings.Split(r.URL.Path, "station/")

	log.Println("urlPathSegments, ", urlPathSegments)

	stationId := urlPathSegments[len(urlPathSegments)-1] // get the last part of array

	log.Println("Station ID:", stationId)

	switch r.Method {
	case http.MethodGet:
		resp, err := natsServer.nc.Request(fmt.Sprintf("Util.About.Station.%s", stationId), nil, 500*time.Millisecond)
		if err != nil {
			responseError(w, http.StatusBadRequest, "Error on request 'Util.About.Station'")
			return
		}

		if resp == nil {
			responseError(w, http.StatusBadRequest, "Problem, has response but no message.")
			return
		}

		log.Println(fmt.Sprintf("Response to request Util.About.Station %s", stationId), string(resp.Data))

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

		log.Println(station)

		responseOk(w, station)
	}
}

func finishTaskHandler(w http.ResponseWriter, r *http.Request) {
	urlPathSegments := strings.Split(r.URL.Path, "finish/")

	log.Println("Finish station work, urlPathSegments, ", urlPathSegments)

	stationId := urlPathSegments[len(urlPathSegments)-1] // get the last part of array

	log.Println("Station ID:", stationId)

	switch r.Method {
	case http.MethodPost:
		err := natsServer.nc.Publish(fmt.Sprintf("Util.Finish.Work.%s", stationId), nil)
		if err != nil {
			responseError(w, http.StatusBadRequest, fmt.Sprintf("Problem, publishing Util.Clear.Available.Work,", err))
			return
		}
		type response struct {
			Message string `json:"message"`
		}
		responseOk(w, response{Message: fmt.Sprintf("Finish current job command has been sent for station %s", stationId)})
	}
}

func publishAvailableTaskHandler(w http.ResponseWriter, r *http.Request) {
	switch r.Method {
	case http.MethodPost:

		log.Println("Publishing available works")

		err := natsServer.nc.Publish("Publish.Available.Work", nil)
		if err != nil {
			responseError(w, http.StatusBadRequest, fmt.Sprintf("Problem publish available works.", err))
			return
		}
		type response struct {
			Message string `json:"message"`
		}
		responseOk(w, response{Message: "Published available works"})
	}
}

func dispatcherHandler(w http.ResponseWriter, r *http.Request) {
	urlPathSegments := strings.Split(r.URL.Path, "dispatcher/")
	value := urlPathSegments[len(urlPathSegments)-1] // get the last part of array

	switch r.Method {
	case http.MethodGet:
		switch value {
		case "db":
			resp, err := natsServer.nc.Request("Util.Dispatcher.Database", nil, 500*time.Millisecond)
			if err != nil {
				responseError(w, http.StatusBadRequest, "Error on request 'Util.Dispatcher.Database'")
				return
			}

			if resp == nil {
				responseError(w, http.StatusBadRequest, "Problem, has response but no message.")
				return
			}

			//log.Println("Response to request Util.Dispatcher.Database\n\t", string(resp.Data))

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
				log.Println("Error on unmarshal task list", err)
			}

			log.Println(tasks)

			responseOk(w, tasks)

		case "clear":
			natsServer.nc.Publish("Util.Dispatcher.Clear", nil)
		}

	}
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

	http.HandleFunc("/task", taskHandler)
	http.HandleFunc("/tasks", taskListHandler)
	http.HandleFunc("/publish", publishAvailableTaskHandler)
	http.HandleFunc("/clear", clearTasksHandler)
	http.HandleFunc("/station/", stationHandler)
	http.HandleFunc("/finish/", finishTaskHandler)
	http.HandleFunc("/dispatcher/db", dispatcherHandler)
	http.HandleFunc("/dispatcher/clear", dispatcherHandler)

	log.Printf("====== Generator server listening on port %s...", serverPort)
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
