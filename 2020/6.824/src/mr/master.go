package mr

import "log"
import "net"
import "os"
import "net/rpc"
import "net/http"
import "time"
import "fmt"
import "sync"
import "sort"
import "encoding/json"

// set log to file
func init() {
    file, err := os.OpenFile("logs.txt", os.O_APPEND|os.O_CREATE|os.O_WRONLY, 0666)
    if err != nil {
        log.Fatal(err)
    }
    log.SetOutput(file)
}

// for sorting by key.
type ByKey []KeyValue

// for sorting by key.
func (a ByKey) Len() int           { return len(a) }
func (a ByKey) Swap(i, j int)      { a[i], a[j] = a[j], a[i] }
func (a ByKey) Less(i, j int) bool { return a[i].Key < a[j].Key }

type taskAssigned struct {
    assigned bool
    mx sync.Mutex
}

type taskStartTime struct {
    start  time.Time
    mx  sync.Mutex
}

func (t *taskStartTime) isTimeOut() bool {
    t.mx.Lock()
    defer t.mx.Unlock()
    timeout := t.start.Add(time.Duration(10)*time.Second)
    now := time.Now()
    if timeout.After(now) {
        return false
    }
    return true
}

func (t* taskStartTime) setTime() {
    t.mx.Lock()
    defer t.mx.Unlock()
    t.start = time.Now()
}

type taskCompleted struct {
    completed bool
    mx sync.Mutex
}

type completedCount struct {
    count int
    mx sync.Mutex
}

func (c *completedCount) inCrease() {
    c.mx.Lock()
    defer c.mx.Unlock()
    c.count++
}

func (c *completedCount) getCount() int {
    c.mx.Lock()
    defer c.mx.Unlock()
    res := c.count
    return res
}
type Master struct {
	// Your definitions here.
    taskTimeout  time.Duration
	numReduce int             // number of reduce task
	numMap int                // number of map task
    files []string            // input filename
    mapTaskAssigned  map[int]*taskAssigned
    reduceTaskAssigned  map[int]*taskAssigned
    mapTaskStart   map[int]*taskStartTime
    reduceTaskStart   map[int]*taskStartTime
    mapTaskCompleted map[int]*taskCompleted
    reduceTaskCompleted map[int]*taskCompleted
    mapTasksCompleted bool
    reduceTasksCompleted bool
    reduceCounter *completedCount
}

// Your code here -- RPC handlers for the worker to call.


func (m *Master) AskReduceTask(args *AskReduceTaskArgs, reply *AskReduceTaskReply) error {
    taskId := m.assignTask2Reduce()
    reply.Success = true
    if taskId == -1 {
        log.Printf("Failed to assign task to reduce")
        reply.Success = false
        return nil
    }
    fileName := fmt.Sprintf("mr-reduce-%d", taskId)
    reply.FileName = fileName
    reply.TaskId = taskId
    return nil
}

func (m *Master) assignTask2Reduce() int {
    // find unassigned reduce task or timeout task
    taskId := -1
    for key, val  := range m.reduceTaskAssigned {
        if m.reduceTaskCompleted[key].completed == true {
            continue
        }
        val.mx.Lock()
        if val.assigned == false {
            taskId = key
            val.assigned = true
            val.mx.Unlock()
            m.reduceTaskStart[key].setTime()
            log.Printf("Assign a task to reduce first time, taskId= %d", taskId)
            break
        }
        val.mx.Unlock()
        start := m.reduceTaskStart[key]
        if start.isTimeOut() {
            taskId = key
            m.reduceTaskStart[key].setTime()
            log.Printf("Assign a task to reduce after timeout, taskId= %d", taskId)
        }
    }
    if taskId == -1 {
        log.Printf("Failed to assigned a task to reduce")
    }
    return taskId
}

func (m *Master) TellMapTaskCompleted(args *TellMapTaskCompletedArgs, reply *TellMapTaskCompletedReply) error {
    taskId := args.TaskId
    if start := m.mapTaskStart[taskId]; start.isTimeOut() {
        log.Printf("Tell Map Completed, but time out")
		return nil
	}
    task := m.mapTaskCompleted[taskId]
    if task.completed == true {
        return nil
    }
    task.mx.Lock()
    task.completed = true
    task.mx.Unlock()
    log.Printf("Mark the Map Task Completed in teller, TaskId= %v", taskId)
    return nil
}

func (m *Master) TellReduceTaskCompleted(args *TellReduceTaskCompletedArgs, reply *TellReduceTaskCompletedReply) error {
    taskId := args.TaskId
    if start := m.reduceTaskStart[taskId]; start.isTimeOut() {
        return nil
    }
    task := m.reduceTaskCompleted[taskId]
	if task.completed == true {
		return nil
	}
    task.mx.Lock()
    task.completed = true
    task.mx.Unlock()
    log.Printf("Mark the Reduce Task Completed in teller, TaskId= %v", taskId)
    reduceFileName := fmt.Sprintf("mr-reduce-%d", taskId)
    os.Remove(reduceFileName)
    m.reduceCounter.inCrease()
    if m.reduceCounter.getCount() == m.numReduce {
        m.reduceTasksCompleted = true
    }
    return nil
}

// if all map tasks completed
func (m *Master) QueryMapTasksCompleted(args *QueryMapTasksCompletedArgs, reply *QueryMapTasksCompletedReply) error {
    reply.AllCompleted = m.isMapTasksCompleted()
    return nil
}

func (m* Master) isMapTasksCompleted() bool {
    if m.mapTasksCompleted == true {
        return true
    }
    for _, val := range m.mapTaskCompleted {
        val.mx.Lock()
        if val.completed == false {
            val.mx.Unlock()
            return false
        }
        val.mx.Unlock()
    }
    m.mapTasksCompleted = true
    // only execute once
    m.sortTempFile()
    return true
}

// if all map tasks completed
func (m *Master) QueryReduceTasksCompleted(args *QueryReduceTasksCompletedArgs, reply *QueryReduceTasksCompletedReply) error {
    reply.AllCompleted = m.isReduceTasksCompleted()
    return nil
}

func (m* Master) isReduceTasksCompleted() bool {
    if m.reduceTasksCompleted == true {
        return true
    }
    for _, val := range m.reduceTaskCompleted {
        val.mx.Lock()
        if val.completed == false {
            val.mx.Unlock()
            return false
        }
        val.mx.Unlock()
    }
    m.reduceTasksCompleted = true
    return true
}

func (m *Master) sortTempFile() {
    for i := 0; i < m.numReduce; i++ {
        reduceFileName := fmt.Sprintf("mr-reduce-%d", i)
        reduceFile, err := os.OpenFile(reduceFileName, os.O_CREATE|os.O_WRONLY, 0666)
        if err != nil {
            log.Fatalf("Can not open or create reduce file, FileName= %v, err= %v", reduceFile, err)
        }
        enc := json.NewEncoder(reduceFile)
        kva := []KeyValue{}
        for j := 0; j < m.numMap; j++ {
            fileName := fmt.Sprintf("mr-%d-%d", j, i)
            file, err := os.Open(fileName)
            if err != nil {
                continue
            }
            dec := json.NewDecoder(file)
            for {
                var kv KeyValue
                if err := dec.Decode(&kv); err != nil {
                    break
                }
                kva = append(kva, kv)
            }
            file.Close()
        }
        sort.Sort(ByKey(kva))
        for _, kv := range kva {
            enc.Encode(&kv)
        }
        reduceFile.Close()
    }
}

func (m *Master) assignTask2Map() int {
    // find unassigned map task or timeout task
    taskId := -1
    for key, val  := range m.mapTaskAssigned {
        if m.mapTaskCompleted[key].completed == true {
            continue
        }
        val.mx.Lock()
        if val.assigned == false {
            taskId = key
            val.assigned = true
            val.mx.Unlock()
            m.mapTaskStart[key].setTime()
            log.Printf("Assign a task to map first time, taskId= %d", taskId)
            break
        }
        val.mx.Unlock()
        start := m.mapTaskStart[key]
        if start.isTimeOut() {
            taskId = key
            m.mapTaskStart[key].setTime()
            log.Printf("Assign a task to map after timeout, taskId= %d", taskId)
        }
    }
    if taskId == -1 {
        log.Printf("Failed to assigned a task to map")
    }
    return taskId
}

// respond with the file name of an as-yet-unstarted map task
func (m *Master) AskMapTask(args *AskMapTaskArgs, reply *AskMapTaskReply) error {
    reply.Success = true
    allCompleted := m.isMapTasksCompleted()
    if allCompleted == true {
        reply.Success = false
        return nil
    }
    taskId := m.assignTask2Map()
    if taskId == -1 {
        reply.Success = false
        return nil
    }
    reply.TaskId = taskId
    reply.FileName = m.files[taskId]
    reply.NumReduce = m.numReduce
    return nil
}

//
// an example RPC handler.
//
// the RPC argument and reply types are defined in rpc.go.
//
func (m *Master) Example(args *ExampleArgs, reply *ExampleReply) error {
	reply.Y = args.X + 1
	return nil
}

func (m *Master ) isMapTaskTimeout(taskId int) bool {
    return false
}

//
// start a thread that listens for RPCs from worker.go
//
func (m *Master) server() {
	rpc.Register(m)
	rpc.HandleHTTP()
	//l, e := net.Listen("tcp", ":1234")
	sockname := masterSock()
	os.Remove(sockname)
	l, e := net.Listen("unix", sockname)
	if e != nil {
		log.Fatal("listen error:", e)
	}
	go http.Serve(l, nil)
}

//
// main/mrmaster.go calls Done() periodically to find out
// if the entire job has finished.
//
func (m *Master) Done() bool {
	ret := false

	// Your code here.
    if m.mapTasksCompleted && m.reduceTasksCompleted {
        ret = true
    }
	return ret
}

//
// create a Master.
// main/mrmaster.go calls this function.
// nReduce is the number of reduce tasks to use.
//
func MakeMaster(files []string, nReduce int) *Master {
	m := Master{}

	// Your code here.
    m.taskTimeout = 10 * time.Second
    m.files = files
    m.numMap = len(m.files)
    m.numReduce = nReduce
    m.mapTaskAssigned = make(map[int]*taskAssigned)
    m.reduceTaskAssigned = make(map[int]*taskAssigned)
    m.mapTaskStart = make(map[int]*taskStartTime)
    m.reduceTaskStart = make(map[int]*taskStartTime)
    m.mapTaskCompleted = make(map[int]*taskCompleted)
    m.reduceTaskCompleted = make(map[int]*taskCompleted)
    m.mapTasksCompleted = false
    m.reduceTasksCompleted = false
    m.reduceCounter = &completedCount{count: 0}

    for i := 0; i < m.numMap; i++ {
        m.mapTaskAssigned[i] = &taskAssigned{assigned: false}
        m.mapTaskCompleted[i] = &taskCompleted{completed: false}
        m.mapTaskStart[i] = &taskStartTime{start: time.Now()}         // just initialize, not really start
    }

    for i := 0; i < m.numReduce; i++ {
        m.reduceTaskAssigned[i] = &taskAssigned{assigned: false}
        m.reduceTaskCompleted[i] = &taskCompleted{completed: false}
        m.reduceTaskStart[i] = &taskStartTime{start: time.Now()}      // just initialize
    }
	m.server()
	return &m
}
