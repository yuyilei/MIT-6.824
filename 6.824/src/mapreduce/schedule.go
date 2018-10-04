package mapreduce

import (
	"fmt"
	"sync"
)

// schedule starts and waits for all tasks in the given phase (Map or Reduce).
func (mr *Master) schedule(phase jobPhase) {
	var ntasks int
	var nios int // number of inputs (for reduce) or outputs (for map)
	             // reduce的输入或者 map的输出，正好是 NumOtherPhase
	switch phase {
	case mapPhase:
		ntasks = len(mr.files)
		nios = mr.nReduce
	case reducePhase:
		ntasks = mr.nReduce
		nios = len(mr.files)
	}

	fmt.Printf("Schedule: %v %v tasks (%d I/Os)\n", ntasks, phase, nios)

	// All ntasks tasks have to be scheduled on workers, and only once all of
	// them have been completed successfully should the function return.
	// Remember that workers may fail, and that any given worker may finish
	// multiple tasks.
	//
	// 所有 ntasks个tasks 必须在调度到 workers上，只有有所的workers完成之后，这个函数才返回
	// wokers可能会失败，并且一个给定的worker可能要完成多个tasks
	// TODO TODO TODO TODO TODO TODO TODO TODO TODO TODO TODO TODO TODO
	//
	// 1. 从channel中取出worker
	// 2. 对worker执行DoTask（通过RPC），如果执行失败把这个放回到channel中
	//
	// 用多个 goroutinue模拟 多个机器，测试分布式系统

	var goroutineWaitGroup sync.WaitGroup
	for i := 0 ; i < ntasks ; i++ {
		goroutineWaitGroup.Add(1)
		go func(phase jobPhase, TaskNumber int, NumOtherPhase int) {
			// 一直执行worker，直到成功
			var worker string
			for {
				// 从channel中取出worker
				worker = <-mr.registerChannel
				var arg DoTaskArgs
				arg.Phase = phase
				arg.TaskNumber = TaskNumber
				arg.NumOtherPhase = NumOtherPhase
				arg.JobName = mr.jobName
				arg.File = mr.files[TaskNumber]
				reply := new(struct{})
				// 使用call（）传递RPC信息
				ok := call(worker,"Worker.DoTask",&arg,&reply)
				// 容错处理，如果master对于worker的RPC调用失败，那么master应该重写分配任务到其他worker。
				// 所以下一次循环再从channel中取出一个 worker，进行RPC调用
				if ok {
					// 工作成功，回收 worker
					// 计数器减一
					goroutineWaitGroup.Done()
					mr.registerChannel <- worker
					// 必须先将计数器减一，再回收worker
					// 因为将worker塞到channel里面可能会阻塞，因为没有goroutine消费它，然后函数就返回不了了
					return
				}
				// 否则就是工作失败
			}
		}(phase,i,nios)
	}

	// 阻塞，直到计数器中的数字减到0
	goroutineWaitGroup.Wait()

	fmt.Printf("Schedule: %v phase done\n", phase)
}
