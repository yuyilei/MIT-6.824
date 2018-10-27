# Mutual Exclusion

分布式系统的基础是多进程之间的并发与协作，这就不可避免的涉及到多个进程对共享资源的互斥访问。以下，是一些分布式互斥算法。

## Centralized Algorithm

集中式算法：仿照单机处理器系统中的方法，选举一个进程作为协作者，有这个协作者调度对共享资源的访问。

当一个进程要访问共享资源，它要向协作者发送一个请求信息，说明它想要访问哪个资源。如果当前没有其他进程访问资源，协作者就发送准许的应答消息。

特性：
1. 安全的，每次只有一个进程访问共享资源；
2. 基于队列的公平性；
3. 一个完整的协议是，一个进程进入临界区，然后退出；
4. 每个周期3个消息（1个请求，1个授权，1个释放）；

### 选择一个协作者（elections）

The Bully Algorithm：

1. 进程p给所有进程号大于它的进程发送一个election信息；
2. 如果没有进程响应，进程p成为协作者；
3. 如果其中一个上级响应了，它成为协作者，进程p的工作也完成了。

![](https://upload-images.jianshu.io/upload_images/5753761-3f74ab02e9d12fa3.png?imageMogr2/auto-orient/)

![](https://upload-images.jianshu.io/upload_images/5753761-db7ba8b37053829c.png?imageMogr2/auto-orient/strip%7CimageView2/2/w/715)

a. 进程4向上级进程5和进程6发送election信息；
b. 进程5和进程6响应了进程4，进程4退出选举；
c. 进程5发送elections信息给上级，进程6发送election给上级；
d. 进程6响应了进程5，进程5退出选举；
e. 进程6成为协作者，并告诉所有进程。


## Decentralized Algorithm

非集中式，主要体现在不止一个协作者，由多个协作者投票（vote）决定哪个进程可以访问共享资源。

特性：
1. 有n个协作者；
2. 一个进程如果想要访问一个共享资源，需要获得 m (> n/2) 个协作者的同意；
3. 协作者用GRANT或DENY响应一个请求；
4. 协作者奔溃后重启，会忘记已经投过票，然后重新投票，会出现不止一个进程有大于n/2票的情况；
5. 进程的票数不够时，会退回重试，也可能一直不得到资源，造成饥饿；
6. 大量请求访问的节点会影响可用性。

为了防止有很多个进程同时发起访问请求导致谁也无法访问共享资源，发起访问的进程可以把请求带上时间戳，其他协作者按照时间戳的先后顺序进行投票允许，同时请求者还需要在访问结束后通知所有其他协作者访问已释放。

## Totally-Ordered Multicasting（完全有序的多播）

如何保证分布式系统中的各个子系统都按相同的顺序执行一组操作？一个等价的问题就是如何在分布式系统下实现全序组播(totally-ordered mulitcast)。这个可以利用Lamport提出的逻辑时钟实现。

使用进程ID保证不同进程的逻辑时钟的不一致：
L(e) = M*Li(e) + i
（M是进程总数，i是当前进程ID，Li是当前时钟值）

实现细节：
1. 多播操作是所有消息以相同顺序传递给每个接收者。
2. Lamport 细节：
    * 每条消息都使用其发送者的当前逻辑时间作为时间戳；
    * 多播消息也都被发送到消息的发送者（意思是发送者自己也会收到自己发送的消息）；
    * 假设一个发送者发送的所有消息都按照它们发送的顺序被接收，并且没有消息丢失；
    * 进程把接收到的消息放入一个本地的队列中，按照消息的时间戳顺序；
    * 消息的接受者向其他所有进程多播一个ACK（确认消息收到）；
    * 只有在队列头部和所有参与者都认可的情况下才能发送消息？？ （什么叫队列头部认可？）；

实现原理：
1. 收到消息的时间戳低于ACK的时间戳，这是Lamport Clock的特性；
2. 所有的进程的在本地保存的队列都是相同的，保证了实现顺序的一致性；


### Lamport Mutual Exclusion

1. 每个进程保存一个队列，队列中保存着各个进程想要访问critical section的请求，队列根据请求中的Lamport 时间戳排序（ e1 -> e2 可以推导出 T(e1) < T(e2）)

2. 当一个进程想要访问C.S.时，它就发送一个带有时间戳的请求到其他各个进程（包括自己）：
    * 等待其他进程的响应；
    * 如果自己的请求位于其队列的头部并且已收到所有答复，进入C.S.；
    * 退出C.S.时，从队列中删除它的请求，并向每个进程发送释放消息。

3. 对于其他进程：
    * 收到一个请求后，在自己的请求队列（按时间戳排序）中输入请求并回复一个时间戳，这个回复是单播的，只用响应消息的发送者；
    * 收到释放消息后，从自己的请求队列中删除相应的请求；
    * 如果自己的请求位于队列的头部，并且收到了全部其他进程的响应，就进入C.S.

4. 正确性：进程x发送带有时间戳Tx的请求时，它的队列中的请求的时间戳都小于Tx的。

### Ricart & Agrawala Algorithm

1. 也基于Lamport totally ordered clocks；
2. 当一个节点i想进入C.S.，它发送带有时间戳的请求到所有其他节点，当收到n-1个响应时，它进入C.S.；
3. 如果节点j有时间戳更早的请求，它不会响应节点i直到它完成进入C.S.的操作；
4. 每个要发送2*(n-1)个消息，(n-1)个请求，(n-1)个回复。

区别主要是，这里没有给自己发请求。

三种接收者的情况：
1. 如果接收者没有访问资源并且不想访问资源，它就向发送者返回OK消息；
2. 如果接收者正在访问资源，它就不会回复。 而是将请求放入等待队列；
3. 如果接收者也想访问资源，但是还没有这样做，它会比较消息的时间戳和自己将要发送给其他节点的消息中的时间戳，时间戳较小的胜出。

下图：

![](https://upload-images.jianshu.io/upload_images/5753761-6e62e616503b0a26.png?imageMogr2/auto-orient/strip%7CimageView2/2/w/343)

1. 进程0和进程2都想访问同一个共享资源；

![](https://upload-images.jianshu.io/upload_images/5753761-4317f358aff6ba54.png?imageMogr2/auto-orient/strip%7CimageView2/2/w/327)

2. 进程0有最小的时间戳，进程0胜出；

![](https://upload-images.jianshu.io/upload_images/5753761-05b94aa97e2dc962.png?imageMogr2/auto-orient/strip%7CimageView2/2/w/556)

3. 进程0完成之后，发送OK给进程2，进程2现在可以继续；

保证每次都只有一个进程（或者节点）访问C.S.，每个节点中的队列是完全一致的。；

保证不会出现死锁，不会有循环（循环中的节点互相等待其他节点）的情况，因为总会有节点先给对方发送OK（时间戳比较）；

保证不会出现饥饿，如果节点发出请求，它最终一定会被授予权限进入C.S.。
因为每个节点的时间戳会随着接收信息而增长，因此每个请求的时间戳最终都会变为最低时间戳，然后请求被执行。

一个Ricart & Agrawala Algorithm的例子：

1. 进程1，2，3通过T(e) = 10*L(e)+id来计算时间戳，L(e)是逻辑时钟；
2. 初始化时间戳：P1: 421, P2: 112, P3: 143；
3. 
    * R m 表示 Receive message m；
    * B m 表示 Broadcast message m to all other processes；
    * S m to j表示 Send message m to process j；

![](https://upload-images.jianshu.io/upload_images/5753761-cedb5e8bdaf7ff34.png?imageMogr2/auto-orient/)

## Token Ring Algorithm

![](https://upload-images.jianshu.io/upload_images/5753761-cf0c9aef412fc82e.png?imageMogr2/auto-orient/)

1. 将进程组成为环；
2. token沿着 -> 传播；
3. 每次只有一个节点有token；
4. 在一次循环中，每个节点都有机会获得token；
5. token可能会丢失。

## Summary

![](https://upload-images.jianshu.io/upload_images/5753761-f09c9fde7010e46a.png?imageMogr2/auto-orient/)

四种算法的比较：

1. Lamport算法演示了分布式进程如何维护数据结构（优先队列）的一致副本；
2. Ricart & Agrawala's algorithms演示了有用的逻辑时钟；
3. Centralized & ring based algorithms需要更少的消息；
4. 这些算法都不能容忍节点crash或消息丢失。
