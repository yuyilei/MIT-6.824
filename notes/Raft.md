# 一致性算法Raft

Raft 是一种为了管理复制日志的一致性算法。Raft将一致性算法分解成了几个关键模块，例如`领导人选举`、`日志复制`和`安全性`。同时它通过实施一个强一致性来减少需要考虑的状态的数量。

## Raft算法

在Raft中，任何时候一个服务器可以扮演下面角色之一：

- Leader：负责接收客户端的请求，将日志复制到其他节点并告知其他节点何时应用这些日志是安全的；

- Candidate：用于选举Leader的一种角色；

- Follower：负责响应来自Leader或者Candidate的请求，完全被动，不会主动发起请求。

开始时，所有的服务器都是follower，等待leader请求，在一段时间内没有收到请求，它就认定leader宕机了，自己变为candidate，开启一次选举，给自己投票，并向其他服务器发起选举请求，如果它获得了大部分服务器的选票，它就变为一个leader。

选举出一个leader之后，leader负责管理复制日志来实现一致性。leader从客户端接收日志条目，把日志条目复制到其他服务器上，并且当保证安全性的时候告诉其他的服务器应用日志条目到他们的状态机中（将日志应用到状态机就是执行日志所对应的操作）。

流程如下：
1. client向leader发送请求，如（set V = 3），此时leader接收到数据是是uncommitted状态；
2. leader给所有follower发送请求，将数据（set V = 3）复制到follower上，leader等待follower的接收响应；
3. leader收到超过超过半数的接收响应后，leader返回一个ACK给client，确认数据已接收；
4. client收到leader的ACK之后，表明此数据是已提交（committed）状态，leader再向follower发通知告知该 数据状态已提交，此时follower就可以应用日志条目到他们的状态机。

## Raft一致性

为了保证算法的一致性，服务器需要遵守一些规则：

![A condensed summary of the Raft consensus algorithm ](https://upload-images.jianshu.io/upload_images/4440914-2a669928f57dde41.png?imageMogr2/auto-orient/strip%7CimageView2/2/w/1240)

**状态**

保存在所有服务器上的状态：

|参数|说明|
|-------|------|
|currentTerm | 服务器最后一次知道的任期号（初始化为 0，持续递增）|
|votedFor | 在当前获得选票的候选人的 Id (没有的话就是null)|
| log[] | 日志条目集；每一个条目包含一个用户状态机执行的指令，和收到时的任期号 |

所有服务器上经常改变的状态：

|参数|说明|
|---|--|
|commitIndex| 已知的最大的已经被提交的日志条目的索引值(初始化为0，持续递增)|
|lastApplied| 已知的最大的已经被应用到状态机的日志条目的索引值(初始化为0，持续递增)|

leader上经常改变的状态：

|参数|说明|
|--|--|
|nextIndex[]|对于每一次服务器，需要发送给它的下一个日志条目的索引（初始化为leader最后一个日志的索引加一）|
|matchIndex[]|对于每一个服务器，已经复制给他的日志的最高索引值（(初始化为0，持续递增)）|

**AppendEntries RPC**

leader发起附加日志的请求使用的是RPC。

附加日志RPC的内容如下：

|参数|说明|
|--|--|
|term|leader的任期值|
|leaderId|leader的ID|
|prevLogIndex|新日志的前面一条日志的索引值|
|prevLogTerm|新日志的前面一条日志的任期值|
|entries[]|准备附加的日志（如果这是一个heartbeat就为空）|
|leaderCommit|leader的commitIndex|

对于附加日志RPC的响应：

|参数|说明|
|--|--|
|term|接受者的currentTerm，用于leader更新自己的term|
|success|当接受者包含了匹配上 prevLogIndex 和 prevLogTerm 的日志时为true|

附加日志RPC的接受者实现：
1. 如果term < currentTerm，返回false；
2. 如果日志在 prevLogIndex 位置处的日志条目的任期号和 prevLogTerm 不匹配，返回 false；
3. 如果已经存在一个日志条目与新的日志条目冲突（同一索引处，但是任期值不同），用新的日志覆盖它；
4. 附加任何在已有的日志中不存在的条目；
5. 如果leaderCommit > commitIndex，设置commitIndex =
min(leaderCommit, 最后一条日志条目的索引值)。


**RequestVote RPC**

candidate在选举的时候，向其他服务器发送RequestVote RPC，内容如下：

|参数|说明|
|--|--|
|term|candidate的任期值|
|candidateId|candidate的ID|
|lastLogIndex|candidate的最后一个日志条目的索引值|
|lastLogTerm|candidate的最后一个日志条目的任期值|

对于选举RPC的响应：

|参数|说明|
|--|--|
|term|接受者的currentTerm，用于candidate更新自己的term|
|success|当接受者把票投给这个candidate时为true|

选举RPC的接受者实现：
1. 如果term < currentTerm，返回false；
2. 如果 votedFor 为空或者就是自己 candidateId，并且候选人的日志至少和自己一样新，那么就投票给它，返回true；

**规则**

服务器需要遵守一些规则：

所有服务器：
1. 如果commitIndex > lastApplied，增加lastApplied，并将日志条目log[lastApplied]应用到状态机；
2. 如果RPC中的term T > currentTerm，设置currentTerm = T，自己变为一个follower；

follower：
1. 响应来自candidate和leader的RPC；
2. 如果超过一段时间都没有收到领导人的心跳，或者是候选人请求投票，就自己变成候选人，开启下一次选举；

candidate：
1. 在转变成候选人后就立即开始选举过程
    * 增加currentTerm；
    * 投票给自己；
    * 重置election timer；
    * 给其他所有server发送RequestVote RPCs；
2. 如果得到了大部分server的选票，变成为leader；
3. 如果收到新leader的AppendEntries RPC，变为follower；
4. 如果选举时间超时，开启一次新的选举。
    
leader：
1. 一旦成为领导人：发送空的附加日志 RPC（心跳）给其他所有的服务器；在一定时间间隔内不停的重复发送，以阻止跟随者超时；
2. 如果收到client的请求，在本地日志中附加新的日志条目，在条目被应用到状态机后响应client； ？（这里有疑问）
3. 如果对于一个跟随者，如果最后一条日志条目的索引值大于等于 nextIndex，就发送从 nextIndex 开始的所有日志条目（AppendEntries RPC）：
    * 如果成功，更新该follower的nextIndex和matchInde；
    * 如果因为日志不一致而失败，减少nextIndex，再重试；
4. 如果存在N，满足N > commitIndex，并且大多数的matchIndex[i] ≥ N成立，并且log[N].term == currentTerm成立，那么令 commitIndex 等于N 

这是对raft一致性的一个总结：

![](https://upload-images.jianshu.io/upload_images/4440914-b4e42c4c483da41d.png?imageMogr2/auto-orient/strip%7CimageView2/2/w/1240)

|性质|说明|
|--|--|
|选举安全性|每个任期中最多一个leader被选举出来|
|leader只附加性|leader的日志从来不会被重写或覆盖，只能附加|
|日志匹配性|如果两个日志有相同的索引号（index）和任期号（term），就认为从头到这个位置的日志之间的日志全部相同|
|leader完整性|如果某个任期中的某个日志已经被提交了，就认为这个日志一定会出现在更大任期的leader的日志中|
|状态机安全性|如果leader已经应用了一个给定索引位置的日志条目在状态机中，那么任何其他的server不能再应用相同索引位置的日志条目|

## Raft实现

一个 Raft 集群包含若干个服务器节点；通常是 5 个，这允许整个系统容忍 2 个节点的失效。在任何时刻，每一个服务器节点都处于这三个状态之一：leader、follower或candidate。

### 领导人选举

如果一个follower在一段时间里没有接收到任何消息，也就是选举超时，那么他就会认为系统中没有可用的leader,并且发起选举以选出新的leader。

在选举期间，candidate可能会收到来自别的candidate或者leader的消息，如果消息中的任期号（term）比自己的term大，这个candidate自动降级为follower，否则（任期号比自己的小），继续保持candidate状态。

投票根据“先来先的”原则，一个server收到一个请求投票的消息，如果消息中的任期号（term）比自己的term大，且candidate的日志比自己的新或一致，就投票给它，并更新自己的term。

这里存在投票限制，server不会投票给日志没有自己新的candidate，来确保选出来的leader包含所有已经提交过的日志。因为赢得选举需要大部分的server的选票，意味着每一个已经提交的日志条目在这些server中肯定存在于至少一个节点上。如果candidate的日志至少和大多数的server一样新，那么他一定持有了所有已经提交的日志条目。

如果多个server同时成为candidate，可能会出现选票被瓜分的情况，在这一轮选票中，没有任何一个candidate成为leader，为了减少出现选票瓜分的情况，Raft 算法使用随机选举超时时间的方法。设置选举超时时间是一个区间内的随机值，把server都分散开，以至于在大多数情况下只有一个server会选举超时。

### 日志复制

leader被选举出来之后，开始为client服务。client的每一个请求都包含一条被复制状态机执行的指令。leader把这条指令作为一条新的日志条目附加到日志中去，然后并行的发起附加条目RPCs给其他的server，让他们复制这条日志条目。当这条日志条目被安全的复制，leader会应用这条日志条目到它的状态机中然后把执行的结果返回给client。如果follower崩溃或者运行缓慢，再或者网络丢包，leader会不断的重复尝试附加日志条目 RPCs （尽管已经回复了client）直到所有的follower都最终存储了所有的日志条目。


leader决定什么时候把一个日志条目应用到状态机是安全的，这种状态叫**可提交**，当leader把某个日志条目复制到大部分的大部分的server上时，这个日志条目就是可以被提交，leader会接下来发送的消息中，更新leaderCommit，别的server一旦知道了某个日志条目可以被提交，就把这个日志条目应用到本地的状态机中。

一个server成为leader的时候，follower的日志可能是各种各样的状态：

![](https://upload-images.jianshu.io/upload_images/4440914-6879710330d40c34.png?imageMogr2/auto-orient/strip%7CimageView2/2/w/1240)

leader处理日志不一致是通过强制follower直接复制自己的日志来解决了。这意味着在follower中的冲突的日志条目会被leader的日志覆盖。

leader维持一个nextIndex[]，保存着对于每一个server，需要发送给它的下一条日志条目的索引。leader初始化nextIndex[]为leader最后一个日志的索引加一。leader在附加日志的RPC有一个prevLogIndex参数，如果follower的日志在 prevLogIndex 位置处的日志条目的任期号和 prevLogTerm 不匹配，则返回 false，leader就会将对应的nextIndex减一，然后发消息重试，最终 nextIndex会在某个位置使得follower和leader的日志达成一致。当这种情况发生，附加日志RPC就会成功，这时就会把follower冲突的日志条目全部删除并且加上leader的日志。一旦附加日志 RPC成功，那么follower的日志就会和leader保持一致，并且在接下来的任期里一直继续保持。


leader不能断定一个之前任期里的日志条目被保存到大多数server上的时候就一定已经提交了，因为一条已经被复制到大部分server上的日志条目上，也可能在新的任期被新的leader的日志条目覆盖，如下图：

![](https://upload-images.jianshu.io/upload_images/4440914-c9b372883ffd4ff0.png?imageMogr2/auto-orient/strip%7CimageView2/2/w/1240)

a. S1是leader，复制日志2到S1，S2；

b. S5是leader（S3，S4，S5投票），复制日志3到S5；

c. S1是leader（S1，S2，S3，S4投票），复制日志2到S3；

d. S5是leader（S2，S3，S4，S5投票），覆盖了S1，S2，S3上的日志2；

Raft不会通过计算副本数目的方式去提交一个前一个任期内的日志条目。只有leader当前任期里的日志条目通过计算副本数目可以被提交；一旦当前任期的日志条目以这种方式被提交，那么由于日志匹配性，之前的日志条目也都会被间接的提交。


### 领导人完整性

领导人完整性：如果某个日志条目在某个任期号中已经被提交，那么这个条目必然出现在更大任期号的所有领导人中。

这是通过投票机制实现的。

Raft 通过比较两份日志中最后一条日志条目的索引值和任期号定义谁的日志比较新。如果两份日志最后的条目的任期号不同，那么任期号大的日志更加新。如果两份日志最后的条目任期号相同，那么日志比较长的那个就更加新。

follower不会给日志没有它新的candidate投票，一个candidate想要成为leader，必须获得大部分follower的选票，也就是它的日志和大部分follower一样新；同时，如果一个日志条目已经被提交，这个日志条目一定被复制到了大部分的server上，所以，这就保证了：leader一定包含之前任期中所有被提交的日志条目。



## Raft Lab 

各个server之间的状态转化：

![](https://upload-images.jianshu.io/upload_images/4440914-b40fe47d0f8118a9.png?imageMogr2/auto-orient/strip%7CimageView2/2/w/1240)

Raft Lab实现的简略图:

![](https://upload-images.jianshu.io/upload_images/4440914-2185e8c2d609b468.png?imageMogr2/auto-orient/strip%7CimageView2/2/w/1240)

具体过程：

一个sever上运行两个goroutinue，一个goroutinue维持三个状态的转变称为goroutinue1；另一个负责向service提交已经submit的日志条目的信息（持久化处理，用于crash后重启），称为goroutinue2。

goroutinue之间的通信用channel。

goroutinue1：

```python
while true：
    if state is follwer:  
        初始化超时时间；
        while true：
            if 接收到投票请求 and 成功投票:
                重置超时时间
            elif 接收到heartbeat and leader的任期大于自己：
                重置超时时间
            elif 接收到AppendEntry and leader的任期大于自己：
                重置超时时间 
            elif 超时：
                become a candidate 
                break 
        
    if state is candidate:
        构造requestVote请求 
        给自己投票
        初始化投票超时时间 
        对其他server广播投票请求
        while true:
            if 接收到heartbeat and leader的任期大于自己：
                become a follower 
                break
            if 接收到AppendEntry and leader的任期大于自己：
                become a follower 
                break 
            if 选举成功：
                become a leader 
                break
            if 选举失败(超时)：
                下一轮选举
                
    if state is leader:
        while state is leader:
            对其他server发送heartbeat或者appendentry
            sleep(一段时间) 
        become a follower 
```


goroutinue2:

```python
while true:
    if 从goutinue2中获得Msg（通过channel）：
        // 发送每一条已经submit到server上，但未提交的日志
        for index in range(LastApplied+1,commitIndex+1):
            ApplyMsg = Msg(Entry[index])
            send ApplyMsg to service (通过channel)
        
        // 更新LastApplied 
        LastApplied = commitIndex 
```


