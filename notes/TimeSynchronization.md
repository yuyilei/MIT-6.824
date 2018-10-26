# Time Synchronization

计算机内部需要时钟同步，一个计算机系统内的计算机也需要同步。否则，当每台机器有自己的时钟时，在另一个事件之后发生的事件可能会被分配一个较早的时间。

但是，两个时钟之间的差异（在任何时刻，没有两个相同的时钟）；
计算机时钟受时钟漂移影响（意思是，慢慢地就不准了）。所以需要时钟同步（Time Synchronization）来保证时钟的相对一致。


## Cristian’s Time Sync

服务器和客户端之间通过二次报文交换，确定主从时间误差，客户端校准本地计算机时间，完成时间同步，有条件的话进一步校准本地时钟频率。


![](https://upload-images.jianshu.io/upload_images/5753761-a6e87064eac4ad16.png?imageMogr2/auto-orient/strip%7CimageView2/2/w/600) 

流程如下：
1. 客户端发送附带T1时间戳的查询报文给服务器；
2. 服务器在该报文上添加到达时刻T2和响应报文发送时刻T3；
3. 客户端记录响应报到达时刻T4。

RTT（Round Trip Time） = (t4-t1)-(t3-t2)
RTT表示从发送到接受到数据包的时间。假设来回网络链路是对称的，即传输时延相等，那么可以计算客户端与服务器之的时间误差D为 (t2-t1)-RTT/2，所以客户端在自己的当前时间加上误差D就可以校准时间。
同时，为了提高准确率，可以发送多次请求，计算RTT的平均值。

## Berkeley algorithm 

Berkeley算法的使用环境与Cristian算法有所不同。Cristian算法是用在一个客户端向一个服务器请求正确时间的时候。而Berkeley算法是几个客户端之间同步时钟的算法。

Berkeley algorithm 是一个同步一组计算机内部的算法。

1. 一个master轮训询问其他slaves的时钟值；
2. master使用往返时间来估计slave的时钟值（同Cristian’s algorithm）
3. Master通过记录RTT的平均值，同时剔除偏差很大的RTT来评估出每个节点的时间偏差。
4. Master发送每个节点的时间偏差到每个节点，让节点自行校正。

（如果master失败，可以选择一个新的master来接管。）

需要注意是，客户端接受到了时间以后，一般来说不会把当前的时间往回调整。因为这会导致一些程序莫名奇妙的错误。因为在很多算法中，时间不会往回调整是一个基本假设。譬如make命令（往回调整时间后，某个目标文件在调整后编译的，则可能比修改后的源文件时间要早）。
解决的方案是：让时钟走慢点。花费一些时间来调整到正确时间。

## Network Time Protocol（NTP）

网络时间协议(Network Time Protocol)，用来同步网络中各个计算机的时间。

同步客户端的时间为国际标准时间UTC（Universal Time Coordinated)。

![](https://upload-images.jianshu.io/upload_images/5753761-eda25108874cba07.png?imageMogr2/auto-orient/strip%7CimageView2/2/w/783)

层次结构:
1. 1类服务器具有高度精确的时钟，如直接连接到原子钟等；
2. 2类服务器只能从第1类和第2类服务器获得时间；
3. 3类服务器从任何服务器获取时间。

同步机制与Cristian算法相似，同步的过程是一个单向的消息传输，而不是消息的往返。

NTP Protocol

1. 使用UDP协议（User Datagram Protocol，不可靠的消息传输协议）
2. 客户端使用Cristian算法同步时钟

Capturejustthe“happensbefore”relationship between events
• Discard the infinitesimal granularity of time • Corresponds roughly to causality
  
                           

## Logical time

前面讨论的时钟都是实际的时间。逻辑时钟，强调的是事件之间的”先后“关系，舍弃时间的微小粒度，大致对应于因果关系。

在许多计算机应用中，只要保证某些事件发生的前后顺序即可，比如make程序时，一个节点编译生成input.o，一个节点是编辑源文件input.c，两个节点判断input.o是否过时（如果input.c的时间在input.o后面，说明需要重新编译），只需要保证 input.o的时间在 input.c 之后，就不需重新编译。

### Lamport Clocks

![](https://upload-images.jianshu.io/upload_images/5753761-ed620931dbf71003.png?imageMogr2/auto-orient/strip%7CimageView2/2/w/766)

1. 如果两个事件在同一个进程pi中，那么它们发生的顺序就是在进程中的顺序，定义为 -> i，如 a -> b 
2. 当一个消息m在两个进程之间传送时，receive(m)一定在发生在send(m)之后，如 b -> c 
3. “先发生”是可传递的
4. 不是所有事件都有 -> 的关系的，比如 a 和 e，它们在不同的进程中，且没有消息传送来连接它们，所以它们是并发的，记为 a || e

对于上图：

1. a -> b(at p1 ); c -> d (at p2)
2. b -> c(m1); d -> f (m2) 

每个进程都保存一个逻辑时钟（软件实现），规则如下：
1. 进程pi有一个逻辑时钟，Li是事件的逻辑时间戳；
2. 进程pi中每发生一个事件，对应的li就加一；
3. 当进程pi发送一个信息m时，都要加上时间戳Li；当进程pj接收信息时，receive(m,t)，设置 Lj = max(Lj,t)，然后 Lj += 1 (规则2)；
4. 进程开始的时候，Li初始化为0；

可以得知：
1. 如果 e1 -> e2，则有 L(e1) < L(e2)，反之，如果 L(e1) < L(e2)，不一定有 e1 -> e2 （可能是 e1 || e2）；
2. 如果 L(e1) == L(e2)，则有 e1 || e2，反之，如果 e1 || e2，不一定 L(e1) == L(e2) 

### Vector Clocks 

为了解决 Lamport Clocks 中 `L(e1) < L(e2)，不一定有 e1 -> e2`  的问题，提出了Vector Clocks。

![](https://upload-images.jianshu.io/upload_images/5753761-5cee40db51e19656.png?imageMogr2/auto-orient/strip%7CimageView2/2/w/653) 

流程如下：
1. 初始化vector[0,0,...,0]
2. 对于进程pi，每发生一个事件，对应的 ci加一 
3. 当进程pi发送一个信息m时，都要加上它的当前vector
4. 当进程pj收到一个信息带有 vector[d1, d2, ..., dn]的m时，更新自己的vector，将本地vector的每个条目设为max(ck,dk)，然后将相对应的 cj加一。 

对于Vector Clocks：
1. e1 -> e2 意味着 V(e1) < V(e2)。 反过来也成立；
2. e1 || e2，如果没有 V(e1) <= V(e2)和 V(e1) >= V(e2)。




