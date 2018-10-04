# MapReduce

MapReduce是一个并行处理和生成大量数据的编程模型。分为Map(映射)和Reduce(规约)两部分。
即使用户不会分布式编程，MapReduce只需要用户提供`map()`函数和`reduce()`函数，就可以实现分布式计算。

## MapReduce原理

简单俩说，MapReduce的实现原理就是“分而治之”。

Map部分主要是“分”，Reduce进行结果的汇总。
比如，我们要统计1000篇论文中每个单词出现次数，Map就是：1号统计第一篇论文，2号统计第二篇...人越多统计得越快；Reduce就是把所有人的统计加在一起，得到最终结果。


## MapReduce实现过程

这是MapReduce的实现过程:

![](http://ohr9krjig.bkt.clouddn.com/Screen%20Shot%202018-10-04%20at%2021.22.01.png)

MapReduce分为两部分：Map和Reduce。集群中机器（worker）分为三种：Master，Mapper，和Reducer。Master负责调度Mapper和Reducer。

1）用户输入文件（提交作业）。MapReduce将用户输入的文件分成M份。

2）Master负责分配map task和reduce task到空闲的Mapper和Reducer上。

3）被分配map task的Mapper会读入用户输入的文件(已被split为M份)。Mapper的工作是对文件的内容调用用户定义的map()函数，会产生大量的中间的key/value对，并存储在内存中。

4）内存中存储的key/value对会被周期性地写到磁盘上，并被分割为R个区域。这些key/value对在磁盘上的位置会返回到Master上，再由Master分配到Reducer上。

5）一个worker被Master分配reduce task之后成为Reducer。Reducer会使用远程的进程调用读取Mapper磁盘上的key/value对的中间文件。Reducer会对读取到的中间文件中的key/value对进行排序（根据key进行排序），这样，拥有一样的key的key/value会被规约到一起。

6）Reducer遍历排序后的中间文件，对于每一个唯一的key和它们的value的集合调用用户定义的reduce()函数，产生的输出被写入到最终输出文件中。

7）所有的Mapper和Reducer都完成工作之后，就表示计算完成了，Master将程序执行权返回给用户。

在整个过程中，用户只需要提供一个map()函数和一个reduce()，其余的交给Master去调度即可。

## 实现MapReduce

Lab1用Golang建立了一个MapReduce库方法，同时学习分布式系统怎么进行容错。
在第一部分编写简单的MapReduce程序（Mapper和Reducer）。在第二部分编写master，master往workers分配任务，处理workers的错误。

注意，MapReduce可以不需要很多机器，在同一台机器上，MapReduce将工作分配到一系列的工作线程（Golang中用goroutine实现）上，这些线程担任Mapper或Reducer，Master则是一个特殊的线程，它调度Mapper和Reducer，为它们分配工作。

### Part One：Map/Reduce输入、输出

这一部分需要实现Mapper和Reducer。

#### 实现Mapper

主要功能：读入一个输入文件，对文件内容调用用户定义的map()函数，将产生的输出写入到n个中间文件中。

对于一个Mapper执行map task需要：

    jobName         // job name of the MapReduce
    mapTaskNumber   // map task 的序号
    inFile          // 输入文件的文件名
    nReduce         // 产生中间文件的数量，也是reduce task的数量
    mapF            // 用户定义的map函数

其中，mapF为func(file string, contents string) []KeyValue，即输入文件名和文件内容，返回key/value的切片。

执行流程：

	1. 打开文件(inFile)，读出全部数据
	2. 调用用户定义的mapF函数，得到 key-value的切片 kv
	3. 创建一系列中间文件（使用reduceName(jobName, mapTaskNumber,r)函数得到文件名）
	4. kv中的数据根据key分类(hash得到索引)，根据（key产生的）索引，将key-value存到不同的文件上，存入文件的格式为json. 
	
注意，要最后再关闭文件，否则不能写入。

#### 实现Reducer

主要功能：读入中间产生的 key/value对(map phase产生的)根据 key 排序 key/value ，对每个key调用用户定义的reduce函数，把输出写到磁盘上。

对于一个Reducer执行reduce task需要：

	jobName           // the name of the whole MapReduce job
	reduceTaskNumber  // which reduce task this is
	nMap              // the number of map tasks that were run即map task的数量
	reduceF           // 用户定义的reduce函数

其中，reduceF为func(key string, values []string) string，即传入 key和这个key对应所有的value的切片，返回规约的结果。

执行流程：

	1. 打开map task产生的文件（reduceName得到文件名）
    2. decode这些文件（文件为json格式，需要decode）。遍历每一个文件，读出每一个key/value对，进行合并，即将拥有一样的key的key/value规约到一起，
	3. 产生一系列文件，文件名从 mergeName得到。
	4. 对合并之后的结果调用reduceF函数，将结果写入文件（注意写入顺序要根据 key进行排序）。
	

### Part Two：Master调度

Master给worker分配task，采用的分布式执行任务。

首先，Master启动rpc服务，并调用schedule函数执行Task。Worker启动后需要调用Master的RPC方法Register注册到Master，而Master发现有了新的Worker，会通知等待channel的协程进行任务操作，(register是一个RPC方法，被worker调用，当worker启动后，用register函数告诉master它能接受tasks)。

同一个worker可能需要处理多个任务，但是不能同时给一个worker分配多个任务。

注意在run中，先做map任务，做完map再做reduce。所以在多个worker做map任务的时候，需要等所有的map任务完成再继续reduce任务，等到所有的reduce执行结束之后，才能返回。

需要注意的点：需要等到所有的worker执行结束，在这里表示所有的goroutine返回，利用`sync.WaitGroup`等待所有的gorouinue返回。

分配worker时，老的worker如果执行完了一次任务，则也要放回channel中以重复使用。

schedule函数执行流程：

	1. 阻塞，直到从channel中取出worker（有空闲的worker）
	2. 对worker执行DoTask（通过RPC），执行结束之后，将worker重新放回到channel中。

其中，schedule函数需要的参数是jobPhase(其实是string)，来决定为worker分配的是map task还是rescue task，以此构造`DoTaskArgs`结构体，其中包括Mapper或者Reducer需要的参数，然后通过RPC对worker执行DoTask（参数为DoTaskArgs结构体），执行map task或者reduce task。

### Part Three：处理worker的错误

Master需要处理worker的错误，这里假设Master不会出错。

因为workers没有可持久化状态，所以MapReduce处理workers的失败相对比较简单。如果一个worker失败了，任何master对它的调用都会失败。因此，如果master对于worker的RPC调用失败，那么master应该重写分配任务到其他worker。

一个RPC的失败并不一定就意味着worker失败，也许worker只是现在不可达，但是它依然还在执行计算。因此，可能发生两个worker接受到同样的任务然后进行计算。但是，因为任务是`幂等`的，所以同一个任务计算两遍也没有关系，即对同一个任务的多次计算得到的结果是相同的。

对worker的容错处理实现起来比较简单，在每个任务执行时加个for循环，如果成功则退出（退出之前需要回收worker，即上worker重新加入channel），否则重新取worker执行任务。

需要注意的点：每一次执行成功退出之前，需要回收worker，也需要对`ync.WaitGroup`的计数器减一，必须先将计数器减一，再回收worker，因为将worker塞到channel里面可能会阻塞，因为没有goroutine消费它，然后函数就返回不了了。





