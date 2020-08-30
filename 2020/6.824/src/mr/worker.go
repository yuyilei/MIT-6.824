package mr

import "fmt"
import "log"
import "os"
import "net/rpc"
import "hash/fnv"
import "io/ioutil"
import "encoding/json"
import "time"

//
// Map functions return a slice of KeyValue.
//
type KeyValue struct {
	Key   string
	Value string
}

//
// use ihash(key) % NReduce to choose the reduce
// task number for each KeyValue emitted by Map.
//
func ihash(key string) int {
	h := fnv.New32a()
	h.Write([]byte(key))
	return int(h.Sum32() & 0x7fffffff)
}


//
// main/mrworker.go calls this function.
//
func Worker(mapf func(string, string) []KeyValue,
	reducef func(string, []string) string) {

	// Your worker implementation here.
    // if not all map task completed, try to query map task all the time
    for ; !QueryMapTasksCompleted(); {
        filename, taskId, nReduce := AskMapTask()
        if filename == nil {
            log.Printf("Can not get map task, sleep 1 seconds")
            time.Sleep(1000*time.Millisecond)
            continue
        }
        log.Printf("Start to do map task, taskId= %v, FileName= %v ", taskId, *filename)
        err := DoMapper(mapf, *filename, taskId, nReduce)
        if err == nil {
            log.Printf("Finish the map task, taskId= %v, then tell master", taskId)
            TellMapTaskCompleted(taskId)
        }
    }

	for ; !QueryReduceTasksCompleted(); {
        filename, taskId := AskReduceTask()
        if filename == nil {
            log.Printf("Can not get reduce task, sleep 1 seconds")
            time.Sleep(1000*time.Millisecond)
            continue
        }
        log.Printf("Start to do reduce task, taskId= %v, FileName= %v", taskId, *filename)
        err := DoReducer(reducef, *filename, taskId)
        if err == nil {
            TellReduceTaskCompleted(taskId)
        }
    }
	// uncomment to send the Example RPC to the master.
	// CallExample()

}


func TellMapTaskCompleted(taskId int) error {
    args := TellMapTaskCompletedArgs{TaskId: taskId}
    reply := TellMapTaskCompletedReply{}
    call("Master.TellMapTaskCompleted", &args, &reply)
    return nil
}

func TellReduceTaskCompleted(taskId int) error {
    args := TellReduceTaskCompletedArgs{TaskId: taskId}
    reply := TellReduceTaskCompletedReply{}
    call("Master.TellReduceTaskCompleted", &args, &reply)
    return nil
}

func DoMapper(mapf func(string, string) []KeyValue, fileName string, taskId int, nReduce int) error {
    file, err := os.Open(fileName)
    if err != nil {
        log.Fatalf("Can not open input file, FileName= %v, err= %v", fileName, err)
        return err
    }
    content, err := ioutil.ReadAll(file)
    if err != nil {
        log.Fatalf("Can not read input file, FileName= %v, err= %v", fileName, err)
    }
    file.Close()
    for i := 0; i < nReduce; i++ {
        oldTempFileName := fmt.Sprintf("mr-%d-%d", taskId, i)
        err := os.Remove(oldTempFileName)
        if err == nil {
            log.Printf("Delete old tempFile, FileName= %v", oldTempFileName)
        }
    }
    kva := mapf(fileName, string(content))
    tempKva := make(map[int][]KeyValue)
    for _, val := range kva {
        y := ihash(val.Key) % nReduce
        tempKva[y] = append(tempKva[y], val)
    }
    for i := 0; i < nReduce; i++ {
        outFileName := fmt.Sprintf("mr-%d-%d", taskId, i)
        tempFileNamePrefix := fmt.Sprintf("mr-map-%d", taskId)
        tempFile, err := ioutil.TempFile(".", tempFileNamePrefix)
        if err != nil {
             log.Printf("Create temp file failed in mapper, taskId= %d", taskId)
        }
        enc := json.NewEncoder(tempFile)
        for _, val := range tempKva[i] {
            err = enc.Encode(&val)
            if err != nil {
                log.Fatalf("Can not write temp file, FileName= %v err= %v", tempFile.Name(), err)
            }
        }
        err = os.Rename(tempFile.Name(), outFileName)
        if err != nil {
            log.Fatalf("Rename temp to target failed in mapper, targetFile= %v, error= %v", outFileName, err)
        }
        tempFile.Close()
    }
    return nil
}

func DoReducer(reducef func(string, []string) string, fileName string, taskId int) error {
    file, err := os.Open(fileName)
    if err != nil {
        log.Fatalf("Can not open temp file, FileName= %v, err= %v", fileName, err)
        return err
    }
    dec := json.NewDecoder(file)
    kva := []KeyValue{}
    for {
        var kv KeyValue
        if err := dec.Decode(&kv); err != nil {
        break
        }
        kva = append(kva, kv)
    }
    file.Close()
    outFileName := fmt.Sprintf("mr-out-%d", taskId)
    err = os.Remove(outFileName)
    if err == nil {
        log.Printf("Delete old outFile, FileName= %d", outFileName)
    }
    tempKva := make(map[string]string)
    for i := 0; i < len(kva); {
        j := i
        values := []string{}
        for j < len(kva) && kva[j].Key == kva[i].Key {
            values = append(values, kva[j].Value)
            j++
        }
        output := reducef(kva[i].Key, values)
        tempKva[kva[i].Key] = output
        i = j
    }
    tempFileNamePrefix := fmt.Sprintf("temp-reduce-%d",taskId)
    tempFile, err := ioutil.TempFile(".", tempFileNamePrefix)
    if err != nil {
        log.Printf("Create temp file failed in reducer, taskId= %d", taskId)
    }
    for key, val := range tempKva {
        fmt.Fprintf(tempFile, "%v %v\n", key, val)
    }
    err = os.Rename(tempFile.Name(), outFileName)
    if err != nil {
        log.Fatalf("Rename temp to target failed in reducer, targetFile= %v, error= %v", outFileName, err)
    }
    tempFile.Close()
    return nil
}

func QueryMapTasksCompleted() bool {
	args := QueryMapTasksCompletedArgs{}
	reply := QueryMapTasksCompletedReply{}
	call("Master.QueryMapTasksCompleted", &args, &reply)
    return reply.AllCompleted
}

func QueryReduceTasksCompleted() bool {
	args := QueryReduceTasksCompletedArgs{}
	reply := QueryReduceTasksCompletedReply{}
	call("Master.QueryReduceTasksCompleted", &args, &reply)
    return reply.AllCompleted
}

func AskMapTask() (*string, int, int) {
    args := AskMapTaskArgs{}
    reply := AskMapTaskReply{}
    call("Master.AskMapTask", &args, &reply)
    if !reply.Success {
        return nil, 0, 0
    } else {
        log.Printf("AskMapTask success, reply= %v", reply)
        return &reply.FileName, reply.TaskId, reply.NumReduce
    }
}

func AskReduceTask() (*string, int) {
    args := AskReduceTaskArgs{}
    reply := AskReduceTaskReply{}
    call("Master.AskReduceTask", &args, &reply)
    if !reply.Success {
        return nil, 0
    } else {
        log.Printf("AskReduceTask success, reply= %v", reply)
        return &reply.FileName, reply.TaskId
    }
}

//
// example function to show how to make an RPC call to the master.
//
// the RPC argument and reply types are defined in rpc.go.
//
func CallExample() {

	// declare an argument structure.
	args := ExampleArgs{}

	// fill in the argument(s).
	args.X = 99

	// declare a reply structure.
	reply := ExampleReply{}

	// send the RPC request, wait for the reply.
	call("Master.Example", &args, &reply)

	// reply.Y should be 100.
	fmt.Printf("reply.Y %v\n", reply.Y)
}

//
// send an RPC request to the master, wait for the response.
// usually returns true.
// returns false if something goes wrong.
//
func call(rpcname string, args interface{}, reply interface{}) bool {
	// c, err := rpc.DialHTTP("tcp", "127.0.0.1"+":1234")
	sockname := masterSock()
	c, err := rpc.DialHTTP("unix", sockname)
	if err != nil {
		log.Fatal("dialing:", err)
	}
	defer c.Close()

	err = c.Call(rpcname, args, reply)
	if err == nil {
		return true
	}

	fmt.Println(err)
	return false
}
