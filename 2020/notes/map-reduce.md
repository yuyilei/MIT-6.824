# Map Reduce 

本实验需要实现`mr`目录下的`master.go`和`worker.go`。

`master.go`内实现master的功能，为map和reduce分配task。要求master需要等到所有的map task都执行完了之后再分配reduce任务，并且所有task都有一个超时时间，当分配给worker的时间超过了超时时间，这个task就会被分配给新的worker执行(这里的超时时间为10秒)。

`worker.gp`内实现map和reduce的功能，map功能为向master所要task，并对task执行用户定义的mapF函数，再将task执行结果返回； reduce功能为向master索要task，并对task执行用户定义的reduceF，并将结果返回给master。用户定义的mapF和reduceF有可能执行过程中crash，所以需要map和reduce能hold住，在map和reduce的过程中实现对文件的原子写入即可（也就是一个map或者reduce只有在保证其mapF或reduceF执行成功后在开始写文件，这样可以保证所有的task执行失败时不影响文件内容）。 

# Master 

```Go
type Master struct {

    taskTimeout  time.Duration
    numReduce int             // reduce task的个数
    numMap int                // map task的个数
    files []string            // 输入文件的slice，长度等于numMap 
    mapTaskAssigned  map[int]*taskAssigned     // 标记每个map task是否被分配给map，taskAssigned结构体内有锁
    reduceTaskAssigned  map[int]*taskAssigned  // 标记每个reduce task是否被分配给reduce
    mapTaskStart   map[int]*taskStartTime      // 记录每个map task开始的时间，taskStartTime结构体内有锁
    reduceTaskStart   map[int]*taskStartTime   // 标记每个reduce task开始的时间
    mapTaskCompleted map[int]*taskCompleted    // 标记每个map task是否结束，taskCompleted结构体内有锁
    reduceTaskCompleted map[int]*taskCompleted  // 标记每个reduce task是否结束 
    mapTasksCompleted bool                      // 标记全部map task是否结束 
    reduceTasksCompleted bool                   // 标记全部reduce task是否结束
    reduceCounter *completedCount               // 记录reduce map完成的个数，内有lock
}

```

master主要实现各种grpc的server端，如，worker索要map task、worker索要reduce task、worker查询是否所有task已完成等。 

以worker索要map task为例（worker索要reduce task同理）

```Go
// respond with the file name of an as-yet-unstarted map task
func (m *Master) AskMapTask(args *AskMapTaskArgs, reply *AskMapTaskReply) error {
    reply.Success = true
    allCompleted := m.isMapTasksCompleted()        //  是否所有map task已经完成，如果已经完成，则没有task再分给map
    if allCompleted == true {
        reply.Success = false
        return nil
    }
    taskId := m.assignTask2Map()                   // 查找所有的map task，看是否能找到未被分配的或者已被分配但是超时的task 
    if taskId == -1 {
        reply.Success = false
        return nil
    }
    reply.TaskId = taskId
    reply.FileName = m.files[taskId]
    reply.NumReduce = m.numReduce
    return nil
}

func (m *Master) assignTask2Map() int {
    // find unassigned map task or timeout task
    taskId := -1
    for key, val  := range m.mapTaskAssigned {             // 遍历所有task
        if m.mapTaskCompleted[key].completed == true {     // 对已经完成的task不进行操作
            continue
        }
        val.mx.Lock()
        if val.assigned == false {                         // 如果task还没被分配出去，就把它分配出去
            taskId = key
            val.assigned = true
            val.mx.Unlock()
            m.mapTaskStart[key].setTime()                  // 如果将此task分配出去，需要设置task的start time
            log.Printf("Assign a task to map first time, taskId= %d", taskId)
            break
        }
        val.mx.Unlock()
        start := m.mapTaskStart[key]                       // 如果task已经被分配出去，就查看它有没有超时 
        if start.isTimeOut() {
            taskId = key
            m.mapTaskStart[key].setTime()                  // 再次将此task分配出去，需要更新task的start time
            log.Printf("Assign a task to map after timeout, taskId= %d", taskId)
        }
    }
    if taskId == -1 {
        log.Printf("Failed to assigned a task to map")
    }
    return taskId
}
``` 

在task完成后，worker会告诉master这个task已经完成，需要注意的是，对于超时和完成状态的task，不能设置其状态为完成。以reduce task为例（map task同理）：

```Go 
func (m *Master) TellReduceTaskCompleted(args *TellReduceTaskCompletedArgs, reply *TellReduceTaskCompletedReply) error {
    taskId := args.TaskId
    if start := m.reduceTaskStart[taskId]; start.isTimeOut() {      // 先检查这个task有没有超时，对于master来说，就算超时的worker完成了工作，也不承认
        return nil
    }
    task := m.reduceTaskCompleted[taskId]
	if task.completed == true {                                     // 也不能设置已完成状态的task
		return nil
	}
    task.mx.Lock()
    task.completed = true
    task.mx.Unlock()
    log.Printf("Mark the Reduce Task Completed in teller, TaskId= %v", taskId)
    reduceFileName := fmt.Sprintf("mr-reduce-%d", taskId)
    os.Remove(reduceFileName)
    m.reduceCounter.inCrease()                                      // 增加完成状态的map task的个数
    if m.reduceCounter.getCount() == m.numReduce {                  // 如果全部的reduce task都已经完成了，设置re
        m.reduceTasksCompleted = true
    }
    return nil
}
```


同时，worker会不断的查询map或reduce task的是否全部完成，以worker查询map task为例（reduce task同理） 


```Go
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
    m.mapTasksCompleted = true                                     // 如果全部map task完成，就设置mapTasksCompleted为true，并对map task产生的file进行排序
    // only execute once
    m.sortTempFile()                                               // 排序只进行一次
    return true
}

func (m *Master) sortTempFile() {                 // 文件 mr-x-y，x表示产生这文件的map task ID， y表示这个key对应的reduce ID（ihash(key) % numReduce），对于每个y，收集所有不同的x所包含的key-value，并进行排序，最后产生很多的 mr-reduce-y文件
    for i := 0; i < m.numReduce; i++ {
        reduceFileName := fmt.Sprintf("mr-reduce-%d", i)
        reduceFile, err := os.OpenFile(reduceFileName, os.O_CREATE|os.O_WRONLY, 0666)
        if err != nil {
            log.Fatalf("Can not open or create reduce file, FileName= %v, err= %v", reduceFile, err)
        }
        enc := json.NewEncoder(reduceFile)
        kva := []KeyValue{}                       // 存储key value 
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
        sort.Sort(ByKey(kva))                   // 通过key进行排序
        for _, kv := range kva {
            enc.Encode(&kv)
        }
        reduceFile.Close()
    }
}
```



# Worker 

Worker承担两部分工作，Mapper和Reducer，在所有map task都完成之后，才开始执行reduce task。 

```Go
func Worker(mapf func(string, string) []KeyValue,
	reducef func(string, []string) string) {

	// Your worker implementation here.
    // if not all map task completed, try to query map task all the time
    for ; !QueryMapTasksCompleted(); {              // 一直向master查询是否所有map task都执行完成，如果没执行完成就一直循环
        filename, taskId, nReduce := AskMapTask()       // 通过rpc向master索要map task
        if filename == nil {                           // 没有索要成功，就sleep 1s
            log.Printf("Can not get map task, sleep 1 seconds")
            time.Sleep(1000*time.Millisecond)
            continue
        }
        log.Printf("Start to do map task, taskId= %v, FileName= %v ", taskId, *filename)
        err := DoMapper(mapf, *filename, taskId, nReduce)        // 索要map task成功，就开始执行Mapper 
        if err == nil {
            log.Printf("Finish the map task, taskId= %v, then tell master", taskId)
            TellMapTaskCompleted(taskId)                        // 执行Mapper成功就告诉master
        }
    }

	for ; !QueryReduceTasksCompleted(); {         // 一直向master查询是否所有reduce task都执行完成，如果没执行完成就一直循环
        filename, taskId := AskReduceTask()             // 通过rpc向master索要map task
        if filename == nil {                           // 没有索要成功，就sleep 1s
            log.Printf("Can not get reduce task, sleep 1 seconds")
            time.Sleep(1000*time.Millisecond)
            continue
        }
        log.Printf("Start to do reduce task, taskId= %v, FileName= %v", taskId, *filename)
        err := DoReducer(reducef, *filename, taskId)          // 索要reduce task成功，就开始执行Reducer
        if err == nil {
            TellReduceTaskCompleted(taskId)                  // 执行Reducer成功就告诉master
        }   
    }
	// uncomment to send the Example RPC to the master.
	// CallExample()

}
```

对于Mapper和Reducer，需要注意的是，用户提供的mapf和reducef有可能会在执行的过程中crash，所以需要将执行mapf或reducef的结果先保存在内存里，等全部的mapf或reducef都执行完，再写入文件，为了实现“原子”写入，在写入文件时，先写入temp 文件，写入完成后，再将temp文件改名。

Mapper实现如下：

```Go
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
        y := ihash(val.Key) % nReduce           // y 表示此 key value需要被哪个Reducer处理
        tempKva[y] = append(tempKva[y], val)    // 将mapf的结果先进行处理，以 ihash(Key) % nReduce 为key，存入map中，这样处理，每个文件可以只用打开一次
    }
    for i := 0; i < nReduce; i++ {              // 每个文件只打开一次
        outFileName := fmt.Sprintf("mr-%d-%d", taskId, i)
        tempFileNamePrefix := fmt.Sprintf("mr-map-%d", taskId)
        tempFile, err := ioutil.TempFile(".", tempFileNamePrefix)           // 先创建temp 文件，等全部成功写入后，在将temp文件改名 
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
```



Reducer如下：

```Go
func DoReducer(reducef func(string, []string) string, fileName string, taskId int) error {
    file, err := os.Open(fileName)
    if err != nil {
        log.Fatalf("Can not open temp file, FileName= %v, err= %v", fileName, err)
        return err
    }
    dec := json.NewDecoder(file)
    kva := []KeyValue{}                      // 把文件中所有的key value先读到内存中
    for {
        var kv KeyValue
        if err := dec.Decode(&kv); err != nil {
        break
        }
        kva = append(kva, kv)
    }
    file.Close()
    outFileName := fmt.Sprintf("mr-out-%d", taskId)
    err = os.Remove(outFileName)             // 如果之前存在对应的mr-out文件，可能是别的Reducer执行此reduce task时留下的，需要删除
    if err == nil {
        log.Printf("Delete old outFile, FileName= %d", outFileName)
    }
    tempKva := make(map[string]string)                     // 将reducef结果暂时保存在内存中，全部完成，再写入文件，避免某次reducef突然crash对结果产生影响
    for i := 0; i < len(kva); {             
        j := i
        values := []string{}
        for j < len(kva) && kva[j].Key == kva[i].Key {     // 整合kva中所有相同的key
            values = append(values, kva[j].Value)   
            j++
        }
        output := reducef(kva[i].Key, values)              
        tempKva[kva[i].Key] = output
        i = j
    }
    tempFileNamePrefix := fmt.Sprintf("temp-reduce-%d",taskId)
    tempFile, err := ioutil.TempFile(".", tempFileNamePrefix)          // 先创建temp 文件，等全部成功写入后，在将temp文件改名
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
```
