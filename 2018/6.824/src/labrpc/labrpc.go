package labrpc

//
// channel-based RPC, for 824 labs.
// allows tests to disconnect RPC connections.
//
// we will use the original labrpc.go to test your code for grading.
// so, while you can modify this code to help you debug, please
// test against the original before submitting.
//
// adapted from Go net/rpc/server.go.
//
// sends gob-encoded values to ensure that RPCs
// don't include references to program objects.
//
// net := MakeNetwork() -- holds network, clients, servers. 网络
// end := net.MakeEnd(endname) -- create a client end-point, to talk to one server. 客户端
// net.AddServer(servername, server) -- adds a named server to network. 在网络中添加一个server
// net.DeleteServer(servername) -- eliminate the named server. 删除一个命名server
// net.Connect(endname, servername) -- connect a client to a server. 连接一个客户端和服务器端
// net.Enable(endname, enabled) -- enable/disable a client. 设置客户端的状态（是否可用 ）
// net.Reliable(bool) -- false means drop/delay messages 是否可用
//
// end.Call("Raft.AppendEntries", &args, &reply) -- send an RPC, wait for reply. 发送一个RPC，等待相应
// the "Raft" is the name of the server struct to be called.   server 的 name
// the "AppendEntries" is the name of the method to be called.  方法的名称
// Call() returns true to indicate that the server executed the request
// and the reply is valid.                                       返回true表示执行成功，且reply有效
// Call() returns false if the network lost the request or reply
// or the server is down.                                        返回false表示没有接受到请求或者server down了
// It is OK to have multiple Call()s in progress at the same time on the
// same ClientEnd.                                               同一个客户端可以同时发起多个call
// Concurrent calls to Call() may be delivered to the server out of order,
// since the network may re-order messages.                      并发的call在服务器端的执行次序可能会被打乱
// Call() is guaranteed to return (perhaps after a delay) *except* if the
// handler function on the server side does not return. That is, there
// is no need to implement your own timeouts around Call().      call 一定会有return，除非这个函数没有return，所以不需要实现 call的 timeout
// the server RPC handler function must declare its args and reply arguments
// as pointers, so that their types exactly match the types of the arguments
// to Call().                                                    函数必须以指针的形势声明参数和返回值
//
// srv := MakeServer()
// srv.AddService(svc) -- a server can have multiple services, e.g. Raft and k/v
//   pass srv to net.AddServer()
//
// svc := MakeService(receiverObject) -- obj's methods will handle RPCs
//   much like Go's rpcs.Register()
//   pass svc to srv.AddService()
//

import "encoding/gob"
import "bytes"
import "reflect"
import "sync"
import "log"
import "strings"
import "math/rand"
import "time"

// 请求信息
type reqMsg struct {
	endname  interface{} // name of sending ClientEnd
	svcMeth  string      // e.g. "Raft.AppendEntries"
	argsType reflect.Type      // 参数类型
	args     []byte            // 参数的切片
	replyCh  chan replyMsg     // 接受回复的通道
}

// 回复消息
type replyMsg struct {
	ok    bool                 // 是否成功
	reply []byte
}

// 客户端节点
type ClientEnd struct {
	endname interface{} // this end-point's name
	ch      chan reqMsg // copy of Network.endCh
}

// send an RPC, wait for the reply.
// the return value indicates success; false means the
// server couldn't be contacted.

// 一个客户端节点发起一个call
func (e *ClientEnd) Call(svcMeth string, args interface{}, reply interface{}) bool {
	req := reqMsg{}
	req.endname = e.endname
	req.svcMeth = svcMeth
	req.argsType = reflect.TypeOf(args)
	req.replyCh = make(chan replyMsg)

	qb := new(bytes.Buffer)
	qe := gob.NewEncoder(qb)
	qe.Encode(args)
	req.args = qb.Bytes()
	// 序列化参数

	e.ch <- req
	// 将构建好的请求加入 客户端节点的通道中

	rep := <-req.replyCh
	// 从 接受回复的通道中取出response

	if rep.ok {
		rb := bytes.NewBuffer(rep.reply)
		rd := gob.NewDecoder(rb)
		// 写入 reply
		if err := rd.Decode(reply); err != nil {
			log.Fatalf("ClientEnd.Call(): decode reply: %v\n", err)
		}
		return true
	} else {
		return false
	}
}

// 记录网络中的服务和客户端
type Network struct {
	mu             sync.Mutex                  // 锁
	reliable       bool                        // 是否可用
	longDelays     bool                        // pause a long time on send on disabled connection 是否暂停一段时间在不可连接之后
	longReordering bool                        // sometimes delay replies a long time 是否重新排序
	ends           map[interface{}]*ClientEnd  // ends, by name
	enabled        map[interface{}]bool        // by end name 是否在网络上可用
	servers        map[interface{}]*Server     // servers, by name
	connections    map[interface{}]interface{} // endname -> servername
	endCh          chan reqMsg                 // 请求信息的通道
}

// 初始化一个网络
func MakeNetwork() *Network {
	rn := &Network{}
	rn.reliable = true                         // 网络可用
	rn.ends = map[interface{}]*ClientEnd{}
	rn.enabled = map[interface{}]bool{}
	rn.servers = map[interface{}]*Server{}
	rn.connections = map[interface{}](interface{}){}
	rn.endCh = make(chan reqMsg)

	// 启动一个 goroutine 处理所有的 calls
	// single goroutine to handle all ClientEnd.Call()s
	go func() {
		for xreq := range rn.endCh {
			// 对于每一个请求，又启动一个 goroutine
			go rn.ProcessReq(xreq)
		}
	}()

	return rn
}

// 加锁，设置网络状态
func (rn *Network) Reliable(yes bool) {
	rn.mu.Lock()
	defer rn.mu.Unlock()

	rn.reliable = yes
}

// 加锁，设置状态
func (rn *Network) LongReordering(yes bool) {
	rn.mu.Lock()
	defer rn.mu.Unlock()

	rn.longReordering = yes
}

func (rn *Network) LongDelays(yes bool) {
	rn.mu.Lock()
	defer rn.mu.Unlock()

	rn.longDelays = yes
}

// 加锁，读取客户端信息
func (rn *Network) ReadEndnameInfo(endname interface{}) (enabled bool,
	servername interface{}, server *Server, reliable bool, longreordering bool,
) {
	rn.mu.Lock()
	defer rn.mu.Unlock()

	enabled = rn.enabled[endname]
	servername = rn.connections[endname]
	if servername != nil {
		server = rn.servers[servername]
	}
	reliable = rn.reliable
	longreordering = rn.longReordering
	return
}

// 加锁，通过endname和servername查看server是不是死了
func (rn *Network) IsServerDead(endname interface{}, servername interface{}, server *Server) bool {
	rn.mu.Lock()
	defer rn.mu.Unlock()

	if rn.enabled[endname] == false || rn.servers[servername] != server {
		return true
	}
	return false
}

// 处理请求
func (rn *Network) ProcessReq(req reqMsg) {
	enabled, servername, server, reliable, longreordering := rn.ReadEndnameInfo(req.endname)
	// 从request中读取信息

	if enabled && servername != nil && server != nil {
		// 如果现在不可用，就随机延迟一段时间
		if reliable == false {
			// short delay
			ms := (rand.Int() % 27)
			time.Sleep(time.Duration(ms) * time.Millisecond)
		}

		// 还是不可用，并且随机等待时间较短？ 就关闭请求，返回超时
		if reliable == false && (rand.Int()%1000) < 100 {
			// drop the request, return as if timeout
			req.replyCh <- replyMsg{false, nil}
			return
		}

		// execute the request (call the RPC handler).
		// in a separate thread so that we can periodically check
		// if the server has been killed and the RPC should get a
		// failure reply.
		// 执行call，在一个单独的请求中，这样可以周期性检查
		// 如果server被杀死了，返回一个错误
		ech := make(chan replyMsg)
		// 启动一个 goroutine
		go func() {
			r := server.dispatch(req)   // server中分发请求
			ech <- r                    // 接受返回信息
		}()

		// wait for handler to return,
		// but stop waiting if DeleteServer() has been called,
		// and return an error.
		// 一直等，等函数的返回
		var reply replyMsg
		replyOK := false
		serverDead := false
		for replyOK == false && serverDead == false {
			select {
			// 请求被处理后，在通道中写入返回信息
			case reply = <-ech:
				replyOK = true
			// 如果通道中一直没有返回信息，就等待一段时间后，检查server是不是死了
			case <-time.After(100 * time.Millisecond):
				serverDead = rn.IsServerDead(req.endname, servername, server)
			}
		}

		// do not reply if DeleteServer() has been called, i.e.
		// the server has been killed. this is needed to avoid
		// situation in which a client gets a positive reply
		// to an Append, but the server persisted the update
		// into the old Persister. config.go is careful to call
		// DeleteServer() before superseding the Persister.
		// 如果server死了，就不必返回，这是为了避免，server已经死了，但客户端还是得到了积极的相应的情况 ？？ 我猜是这个意思
		// 所以最后还是要检查一个server是不是还活着
		serverDead = rn.IsServerDead(req.endname, servername, server)

		if replyOK == false || serverDead == true {
			// server was killed while we were waiting; return error.  等待的时候server被杀死
			req.replyCh <- replyMsg{false, nil}
		} else if reliable == false && (rand.Int()%1000) < 100 {
			// drop the reply, return as if timeout                    超时
			req.replyCh <- replyMsg{false, nil}
		} else if longreordering == true && rand.Intn(900) < 600 {
			// delay the response for a while                          延迟返回
			ms := 200 + rand.Intn(1+rand.Intn(2000))
			time.Sleep(time.Duration(ms) * time.Millisecond)
			req.replyCh <- reply
		} else {
			req.replyCh <- reply
		}
	} else {
		// simulate no reply and eventual timeout.
		// 没有回复和超时
		ms := 0
		if rn.longDelays {
			// let Raft tests check that leader doesn't send
			// RPCs synchronously.
			ms = (rand.Int() % 7000)
		} else {
			// many kv tests require the client to try each
			// server in fairly rapid succession.
			ms = (rand.Int() % 100)
		}
		time.Sleep(time.Duration(ms) * time.Millisecond)
		req.replyCh <- replyMsg{false, nil}
	}

}

// create a client end-point.
// start the thread that listens and delivers.
func (rn *Network) MakeEnd(endname interface{}) *ClientEnd {
	rn.mu.Lock()
	defer rn.mu.Unlock()

	if _, ok := rn.ends[endname]; ok {
		log.Fatalf("MakeEnd: %v already exists\n", endname)
	}

	e := &ClientEnd{}
	e.endname = endname
	e.ch = rn.endCh                    // 通过向e.ch发送消息就是往网络上面发送信息
	rn.ends[endname] = e
	rn.enabled[endname] = false
	rn.connections[endname] = nil

	return e
}


func (rn *Network) AddServer(servername interface{}, rs *Server) {
	rn.mu.Lock()
	defer rn.mu.Unlock()

	rn.servers[servername] = rs
}

func (rn *Network) DeleteServer(servername interface{}) {
	rn.mu.Lock()
	defer rn.mu.Unlock()

	rn.servers[servername] = nil
}

// connect a ClientEnd to a server.
// a ClientEnd can only be connected once in its lifetime.
// 一个客户端只能被连接一次 q
func (rn *Network) Connect(endname interface{}, servername interface{}) {
	rn.mu.Lock()
	defer rn.mu.Unlock()

	rn.connections[endname] = servername
}

// enable/disable a ClientEnd.
// 设置客户端状态
func (rn *Network) Enable(endname interface{}, enabled bool) {
	rn.mu.Lock()
	defer rn.mu.Unlock()

	rn.enabled[endname] = enabled
}

// get a server's count of incoming RPCs.
// 到底是 server的数量还是 RPC的数量 ?
func (rn *Network) GetCount(servername interface{}) int {
	rn.mu.Lock()
	defer rn.mu.Unlock()

	svr := rn.servers[servername]
	return svr.GetCount()
}

//
// a server is a collection of services, all sharing
// the same rpc dispatcher. so that e.g. both a Raft
// and a k/v server can listen to the same rpc endpoint.
// 一个server中的service有同一个分发器！
type Server struct {
	mu       sync.Mutex
	services map[string]*Service          // 名字和 service的键值对
	count    int // incoming RPCs
}

func MakeServer() *Server {
	rs := &Server{}
	rs.services = map[string]*Service{}
	return rs
}

func (rs *Server) AddService(svc *Service) {
	rs.mu.Lock()
	defer rs.mu.Unlock()
	rs.services[svc.name] = svc
}

// 分发请求
func (rs *Server) dispatch(req reqMsg) replyMsg {
	rs.mu.Lock()

	rs.count += 1

	// split Raft.AppendEntries into service and method
	dot := strings.LastIndex(req.svcMeth, ".")
	serviceName := req.svcMeth[:dot]
	methodName := req.svcMeth[dot+1:]

	service, ok := rs.services[serviceName]

	rs.mu.Unlock()

	if ok {
		return service.dispatch(methodName, req)
	} else {
		// 没有这个service的话，返回所有的service以供选择
		choices := []string{}
		for k, _ := range rs.services {
			choices = append(choices, k)
		}
		log.Fatalf("labrpc.Server.dispatch(): unknown service %v in %v.%v; expecting one of %v\n",
			serviceName, serviceName, methodName, choices)
		return replyMsg{false, nil}
	}
}

func (rs *Server) GetCount() int {
	rs.mu.Lock()
	defer rs.mu.Unlock()
	return rs.count
}

// an object with methods that can be called via RPC.
// a single server may have more than one Service.
// 带方法的对象，能被调用
type Service struct {
	name    string                              // server name
	rcvr    reflect.Value
	typ     reflect.Type
	methods map[string]reflect.Method           // name -> method
}

func MakeService(rcvr interface{}) *Service {
	svc := &Service{}
	svc.typ = reflect.TypeOf(rcvr)
	svc.rcvr = reflect.ValueOf(rcvr)
	svc.name = reflect.Indirect(svc.rcvr).Type().Name()
	svc.methods = map[string]reflect.Method{}

	for m := 0; m < svc.typ.NumMethod(); m++ {
		method := svc.typ.Method(m)
		mtype := method.Type
		mname := method.Name

		//fmt.Printf("%v pp %v ni %v 1k %v 2k %v no %v\n",
		//	mname, method.PkgPath, mtype.NumIn(), mtype.In(1).Kind(), mtype.In(2).Kind(), mtype.NumOut())

		if method.PkgPath != "" || // capitalized?
			mtype.NumIn() != 3 ||
			//mtype.In(1).Kind() != reflect.Ptr ||
			mtype.In(2).Kind() != reflect.Ptr ||
			mtype.NumOut() != 0 {
			// the method is not suitable for a handler
			//fmt.Printf("bad method: %v\n", mname)
		} else {
			// the method looks like a handler
			svc.methods[mname] = method
		}
	}

	return svc
}

func (svc *Service) dispatch(methname string, req reqMsg) replyMsg {
	if method, ok := svc.methods[methname]; ok {
		// 根据方法名字，查找方法
		// prepare space into which to read the argument.
		// the Value's type will be a pointer to req.argsType.
		args := reflect.New(req.argsType)

		// decode the argument.
		ab := bytes.NewBuffer(req.args)
		ad := gob.NewDecoder(ab)
		ad.Decode(args.Interface())

		// allocate space for the reply.
		replyType := method.Type.In(2)
		replyType = replyType.Elem()
		replyv := reflect.New(replyType)

		// call the method.
		function := method.Func
		function.Call([]reflect.Value{svc.rcvr, args.Elem(), replyv})

		// encode the reply.
		rb := new(bytes.Buffer)
		re := gob.NewEncoder(rb)
		re.EncodeValue(replyv)

		return replyMsg{true, rb.Bytes()}
	} else {
		choices := []string{}
		for k, _ := range svc.methods {
			choices = append(choices, k)
		}
		log.Fatalf("labrpc.Service.dispatch(): unknown method %v in %v; expecting one of %v\n",
			methname, req.svcMeth, choices)
		return replyMsg{false, nil}
	}
}
