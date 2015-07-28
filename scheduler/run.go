package scheduler

import (
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"scheduler/config"
	"scheduler/queue"
	"scheduler/util"
	"strings"
	"time"

	"github.com/hashicorp/consul/api"
)

func (s *Scheduler) Run() {
	err := s.connect()
	if err != nil {
		panic(err)
	}

	for {
		time.Sleep(1 * time.Second)

		err := s.polling()
		if err != nil {
			fmt.Printf("[Error] %s\n", err)
			continue
		}
	}
}

func (s *Scheduler) polling() error {
	//	Create critical section by consul lock
	l, err := util.Consul().LockKey(LOCK_KEY)
	if err != nil {
		return err
	}
	_, err = l.Lock(nil)
	if err != nil {
		return err
	}
	defer l.Unlock()

	//	Polling tasks from queue
	var eventTasks []EventTask
	pq := &queue.Queue{Client: util.Consul(), Key: PROGRESS_QUEUE_KEY}
	err = pq.Items(&eventTasks)
	if err != nil {
		return err
	}

	task := eventTasks[0]
	switch {
	case len(eventTasks) == 0:
		return s.dispatchEvent()
	case task.Runnable(s.node):
		//	runTask is parallelizable
		l.Unlock()
		return s.runTask(task)
	case task.IsFinished():
		return s.finishTask(task)
	default:
		fmt.Printf("Wait a task will have been finished by other instance(Task: %s, Service: %s, Tag: %s)\n", task.Task, task.Service, task.Tag)
	}
	return nil
}

func (scheduler *Scheduler) connect() error {
	var err error
	scheduler.node, err = os.Hostname()
	if err != nil {
		return err
	}

	return scheduler.registerServer()
}

func (s *Scheduler) registerServer() error {
	var key = "cloudconductor/servers/" + s.node
	var c *api.Client = util.Consul()
	kv, _, err := c.KV().Get(key, &api.QueryOptions{})
	if err != nil {
		return err
	}

	if kv == nil {
		kv = &api.KVPair{Key: key}
	}

	m := make(map[string]interface{})
	m["roles"] = strings.Split(config.Role, ",")
	m["private_ip"], err = getAddress(s.node)
	if err != nil {
		return err
	}

	kv.Value, err = json.Marshal(m)
	if err != nil {
		return err
	}

	_, err = c.KV().Put(kv, &api.WriteOptions{})
	if err != nil {
		return err
	}

	return nil
}

func getAddress(node string) (string, error) {
	nodes, _, err := util.Consul().Catalog().Nodes(&api.QueryOptions{})
	if err != nil {
		return "", err
	}
	for _, n := range nodes {
		if n.Node == node {
			return n.Address, nil
		}
	}

	return "", errors.New("Current node does not found in consul catalog")
}

func (s *Scheduler) dispatchEvent() error {
	pq := &queue.Queue{Client: util.Consul(), Key: PROGRESS_QUEUE_KEY}
	eq := &queue.Queue{Client: util.Consul(), Key: EVENT_QUEUE_KEY}

	var consulEvent api.UserEvent
	err, found := eq.DeQueue(&consulEvent)
	if err != nil {
		return err
	}
	if !found {
		return nil
	}
	fmt.Printf("Dispatch event(ID: %s, Name: %s)\n", consulEvent.ID, consulEvent.Name)

	//	Collect events over all task.yml and dispatch tasks to progress task queue
	events := s.sortedEvents(consulEvent.Name)
	c := 0
	for _, v := range events {
		switch {
		case v.Task != "":
			pq.EnQueue(EventTask{Pattern: v.Pattern, ID: consulEvent.ID, No: c, Task: v.Task})
			c += 1
		case len(v.OrderedTasks) > 0:
			for _, t := range v.OrderedTasks {
				t.Pattern = v.Pattern
				t.ID = consulEvent.ID
				t.No = c
				pq.EnQueue(t)
				c += 1
			}
		}
	}

	//	Log starting event as EventResult on KVS
	result := &EventResult{ID: consulEvent.ID, Name: consulEvent.Name, Status: "inprogress", StartedAt: time.Now()}
	return result.Save()
}

func (s *Scheduler) runTask(task EventTask) error {
	fmt.Printf("Run task(Task: %s, ID: %s, No: %d, Service: %s, Tag: %s)\n", task.Task, task.ID, task.No, task.Service, task.Tag)

	if err := task.WriteStartLog(s.node); err != nil {
		return err
	}

	status := "success"
	if err := task.Run(s); err != nil {
		status = "error"
		fmt.Println("[Error] Following error has occurred while executing task")
		fmt.Println(err)
	}

	return task.WriteFinishLog(s.node, status)
}

func (s *Scheduler) finishTask(task EventTask) error {
	//	Log finishing task as TaskResult on KVS when finished task over all nodes
	nodeResults, err := getNodeTaskResults(task.ID, task.No)
	if err != nil {
		return err
	}
	status := "success"
	for _, nr := range nodeResults {
		if nr.Status == "error" {
			status = "error"
			// remove following tasks in progress task queue when some error has been occurred
			pq := &queue.Queue{Client: util.Consul(), Key: PROGRESS_QUEUE_KEY}
			pq.Clear()
		}
	}
	result, err := getTaskResult(task.ID, task.No)
	if err != nil {
		return err
	}
	if result == nil {
		result = &TaskResult{EventID: task.ID, No: task.No, Name: task.Task, Status: "inprogress", StartedAt: time.Now()}
	}
	result.Status = status
	result.FinishedAt = time.Now()
	err = result.Save()
	if err != nil {
		return err
	}

	//	Dequeue task from task queue when finished task over all all nodes
	pq := &queue.Queue{Client: util.Consul(), Key: PROGRESS_QUEUE_KEY}
	var dummy EventTask
	err, found := pq.DeQueue(&dummy)
	if err != nil || !found {
		return err
	}

	//	Log finishing event as EventResult on KVS when finished all task in a progress task queue
	var tasks []EventTask
	err = pq.Items(&tasks)
	if err != nil {
		return err
	}
	if len(tasks) == 0 {
		eventResult, err := getEventResult(task.ID)
		if err != nil {
			return err
		}
		//	TODO: collect results of all tasks under an event
		eventResult.Status = status
		eventResult.FinishedAt = time.Now()
		err = eventResult.Save()
		if err != nil {
			return err
		}
	}

	return nil
}
