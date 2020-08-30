package mr

//
// RPC definitions.
//
// remember to capitalize all names.
//

import "os"
import "strconv"

//
// example to show how to declare the arguments
// and reply for an RPC.
//

type ExampleArgs struct {
	X int
}

type ExampleReply struct {
	Y int
}

// Add your RPC definitions here.

type AskReduceTaskArgs struct {

}

type AskReduceTaskReply struct {
    FileName string
    TaskId int
    Success bool
}

type TellMapTaskCompletedArgs struct {
    TaskId int
}

type TellMapTaskCompletedReply struct {
    Success bool
}

type TellReduceTaskCompletedArgs struct {
    TaskId int
}

type TellReduceTaskCompletedReply struct {
    Success bool
}

type AskMapTaskArgs struct {

}

type AskMapTaskReply struct {
	FileName string
	TaskId int
    Success bool
	NumReduce int
}

type QueryMapTasksCompletedArgs struct {

}

type QueryMapTasksCompletedReply struct {
    AllCompleted bool
}

type QueryReduceTasksCompletedArgs struct {

}

type QueryReduceTasksCompletedReply struct {
    AllCompleted bool
}

// Cook up a unique-ish UNIX-domain socket name
// in /var/tmp, for the master.
// Can't use the current directory since
// Athena AFS doesn't support UNIX-domain sockets.
func masterSock() string {
	s := "/var/tmp/824-mr-"
	s += strconv.Itoa(os.Getuid())
	return s
}
