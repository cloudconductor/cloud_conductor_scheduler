package scheduler

import "fmt"

type DispatchTask struct {
	Service string
	Tag     string
	Task    string
}

type Event struct {
	Description  string
	Priority     int
	OrderedTasks []DispatchTask `json:"ordered_tasks"`
	Task         string
}

func (e Event) String() string {
	s := ""
	s += fmt.Sprintf("Description: %s\n", e.Description)
	s += fmt.Sprintf("Priority: %d\n", e.Priority)
	if e.Task != "" {
		s += fmt.Sprintf("Task: %s\n", e.Task)
	}

	if len(e.OrderedTasks) > 0 {
		s += "OrderedTasks:\n"
		for i, v := range e.OrderedTasks {
			if v.Tag == "" {
				s += fmt.Sprintf("  %d: Service: %s, Task: %s\n", i, v.Service, v.Task)
			} else {
				s += fmt.Sprintf("  %d: Service: %s, Tag: %s, Task: %s\n", i, v.Service, v.Tag, v.Task)
			}
		}
	}
	return s
}
