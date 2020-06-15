
up:
	docker-compose up --build -d 
.PHONY: up


cluster-info:
	curl http://127.0.0.1:8222/varz
	curl http://127.0.0.1:8222/routez
.PHONY: cluster-info

restart:
	docker stop assigner 
	docker start assigner
.PHONY: restart

tail-assigner:
	docker logs assigner -f
tail-qhandler:
	docker logs qhandler -f
tail-generator:
	docker logs generator -f 
tail-dispatcher:
	docker logs dispatcher -f 
tail-station101:
	docker logs station101 -f 
tail-station201:
	docker logs station201 -f 
tail-station301:
	docker logs station301 -f 
.PHONY: tail-assigner tail-qhandler tail-generator tail-station101 tail-station201 tail-station301 

add-task:
	curl -X POST localhost:7070/task -d '{"ID":"10001", "name":"Bore 5mm hole"}'
	curl -X POST localhost:7070/task -d '{"ID":"10002", "name":"Install 3/4 screw"}'
list-tasks:
	curl localhost:7070/tasks
pub-work:
	curl -X POST localhost:7070/publish
clear-works:
	curl -X POST localhost:7070/clear

# inquire station 101 current info including status	
station-101:
	curl localhost:7070/station/101
station-201:
	curl localhost:7070/station/201
station-301:
	curl localhost:7070/station/301

# station 101 will finish existing work	
finish-101:
	curl -X POST localhost:7070/finish/101
finish-201:
	curl -X POST localhost:7070/finish/201
finish-301:
	curl -X POST localhost:7070/finish/301

# peek on dispatcher all tasks recorded
dispatcher-db:
	curl localhost:7070/dispatcher/db
dispatcher-clear:
	curl localhost:7070/dispatcher/clear
