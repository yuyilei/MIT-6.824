package raft

//
// this is an outline of the API that raft must expose to
// the service (or tester). see comments below for
// each of these functions for more details.
//
// rf = Make(...)
//   create a new Raft server.
// rf.Start(command interface{}) (index, term, isleader)
//   start agreement on a new log entry
// rf.GetState() (term, isLeader)
//   ask a Raft for its current term, and whether it thinks it is leader
// ApplyMsg
//   each time a new entry is committed to the log, each Raft peer
//   should send an ApplyMsg to the service (or tester)
//   in the same server.
//

import "sync"
import (
	"labrpc"
	"math/rand"
	"time"
	"bytes"
	"encoding/gob"
)

// import "bytes"
// import "encoding/gob"


const (
	LEADER = iota
	CANDIDATE
	FOLLOWER
)

//
// as each Raft peer becomes aware that successive log entries are
// committed, the peer should send an ApplyMsg to the service (or
// tester) on the same server, via the applyCh passed to Make().
//
type ApplyMsg struct {
	Index       int
	Command     interface{}
	UseSnapshot bool   // ignore for lab2; only used in lab3
	Snapshot    []byte // ignore for lab2; only used in lab3
}

type Log struct {
	Term int
	Index int
	Command interface{}
}


//
// A Go object implementing a single Raft peer.
//
type Raft struct {
	mu        sync.Mutex
	peers     []*labrpc.ClientEnd
	persister *Persister
	me        int // index into peers[]

	// Your data here.
	// Look at the paper's Figure 2 for a description of what
	// state a Raft server must maintain.

	State int          // 当前状态
	LeadId int         // leader的ID
	VoteCount int      // 收到的票数
	CurrentTerm int    // 当前任期值
	VotedFor int       // 当前获得选票的 candidate
	Entry []Log  	   // 日志条目集
	CommitIndex int    // 已知的最大的已经被提交的日志条目的索引值
	LastApplied int    // 已知的最大的已经被应用到状态机的日志条目的索引值

	// leader 的参数
	NextIndex []int    // 对于每一次服务器，需要发送给它的下一个日志条目的索引
	MatchIndex []int   // 对于每一个服务器，已经复制给他的日志的最高索引值

	Heartbeatchan  chan bool     // 收到heartbeat
	IsLeaderchan   chan bool     // 是否为leader
	VotedForchan   chan bool     // 是否收到投票
	GrantVotechan  chan bool     // 是否给别人投票
	CommitLogchan  chan bool     // 提交log
}

// return currentTerm and whether this server
// believes it is the leader.
func (rf *Raft) GetState() (int, bool) {

	var term int
	var isleader bool

	rf.mu.Lock()
	defer rf.mu.Unlock()
	term, isleader = rf.CurrentTerm, rf.State == LEADER

	return term, isleader
}

//
// save Raft's persistent state to stable storage,
// where it can later be retrieved after a crash and restart.
// see paper's Figure 2 for a description of what should be persistent.
//
func (rf *Raft) persist() {
	// Your code here.
	// Example:
	w := new(bytes.Buffer)
	e := gob.NewEncoder(w)
	e.Encode(rf.Entry)
	e.Encode(rf.CurrentTerm)
	e.Encode(rf.LastApplied)
	e.Encode(rf.CommitIndex)
	e.Encode(rf.VotedFor)
	data := w.Bytes()
	rf.persister.SaveRaftState(data)
}

//
// restore previously persisted state.
//
func (rf *Raft) readPersist(data []byte) {
	// Your code here.
	// Example:
	r := bytes.NewBuffer(data)
	d := gob.NewDecoder(r)
	d.Decode(&rf.Entry)
	d.Decode(&rf.CurrentTerm)
	d.Decode(&rf.LastApplied)
	d.Decode(&rf.CommitIndex)
	d.Decode(&rf.VotedFor)
}

type InstallSnapArgs struct {
	Term  int           // leader的任期号
	LeaderId int        // leader的ID
	LastIncludedIndex int  // 快照中包含的最后日志条目的索引值
	LastIncludedTerm int   // 快照中包含的最后日志条目的任期号
	Offset	int 		   // 分块在快照中的偏移量
	Data    []byte         // 快照数据
	Done    bool           // 如果这是最后一个分块则为 true
}

type InstallSnapReply struct {
	Term int // 当前任期号
}

//
// example RequestVote RPC arguments structure.
//
type RequestVoteArgs struct {
	// Your data here.
	Term int  			// candidate的任期号
	CandidateId int     // candidate的ID
	LastLogIndex  int
	LastLogTerm   int
}

//
// example RequestVote RPC reply structure.
//
type RequestVoteReply struct {
	// Your data here.
	Term  int
	VoteGranted bool
}

//
// example RequestVote RPC handler.
//
func (rf *Raft) RequestVote(args RequestVoteArgs, reply *RequestVoteReply) {
	rf.mu.Lock()
	defer rf.mu.Unlock()
	defer rf.persist()
	reply.Term = rf.CurrentTerm
	reply.VoteGranted = false

	// fmt.Printf("%v %v",rf.CurrentTerm,args.Term)
	if rf.CurrentTerm > args.Term {          // candidate的任期比自己小
		return
	}

	if rf.CurrentTerm < args.Term {          // candidate的任期比自己大，表示新的选举开始
		rf.State = FOLLOWER                   // 变为follower
		rf.VotedFor = -1                     // 取消上一次投票
		rf.CurrentTerm = args.Term           // 修改任期
	}

	if rf.VotedFor == -1 || rf.VotedFor == args.CandidateId {                   // 本轮中还没投票
		lastLogTerm := rf.Entry[len(rf.Entry)-1].Term
		lastLogIndex := rf.Entry[len(rf.Entry)-1].Index

		if ((lastLogTerm < args.LastLogTerm) ||          // 候选者的最后一条日志至少和自己的一样新
			(lastLogTerm == args.LastLogTerm && lastLogIndex <= args.LastLogIndex)) {
				// fmt.Printf("%d Vote for %d\n",rf.me,args.CandidateId)
				reply.VoteGranted = true
				rf.State = FOLLOWER
				rf.VotedFor = args.CandidateId
				rf.GrantVotechan <- true
		}
	}
}

//
// example code to send a RequestVote RPC to a server.
// server is the index of the target server in rf.peers[].
// expects RPC arguments in args.
// fills in *reply with RPC reply, so caller should
// pass &reply.
// the types of the args and reply passed to Call() must be
// the same as the types of the arguments declared in the
// handler function (including whether they are pointers).
//
// returns true if labrpc says the RPC was delivered.
//
// if you're having trouble getting RPC to work, check that you've
// capitalized all field names in structs passed over RPC, and
// that the caller passes the address of the reply struct with &, not
// the struct itself.
//
func (rf *Raft) sendRequestVote(server int, args RequestVoteArgs, reply *RequestVoteReply) bool {
	ok := rf.peers[server].Call("Raft.RequestVote", args, reply)

	if ok {
		rf.mu.Lock()
		defer rf.mu.Unlock()
		if rf.State != CANDIDATE {                 // 不再是候选者
			return ok
		}
		if reply.Term > rf.CurrentTerm {
			rf.CurrentTerm = reply.Term
			rf.State = FOLLOWER
			rf.VoteCount = 0
			rf.VotedFor = -1
			rf.persist()
		}
		if reply.VoteGranted {
			rf.VoteCount += 1
			if rf.VoteCount > len(rf.peers)/2 {
				rf.IsLeaderchan <- true
				rf.State = LEADER
				rf.persist()
			}
		}
	}
	return ok
}

// 广播投票请求
func (rf *Raft) broadcastRequestVote() {
	rf.mu.Lock()
	args := RequestVoteArgs{}
	args.Term = rf.CurrentTerm
	args.CandidateId = rf.me
	args.LastLogIndex = rf.Entry[len(rf.Entry)-1].Index
	args.LastLogTerm = rf.Entry[len(rf.Entry)-1].Term
	rf.mu.Unlock()
	for index := range rf.peers {
		if rf.State == CANDIDATE && index != rf.me {
			go func(index int) {
				reply := RequestVoteReply{}
				rf.sendRequestVote(index, args, &reply)
			}(index)
		}
	}
}


type AppendEntriesArgs struct {
	Term  int
	LeadId int
	PrevLogIndex int
	PrevLogTerm int
	Entries []Log
	LeaderCommit int
}

type AppendEntriesReply struct {
	Term int
	Success bool
	NextLogIndex int
}

func (rf *Raft) AppendEntries(args AppendEntriesArgs, reply *AppendEntriesReply) {
	rf.mu.Lock()
	defer rf.mu.Unlock()
	defer rf.persist()
	reply.Term = rf.CurrentTerm
	reply.Success = false

	if rf.CurrentTerm > args.Term {                 // 任期比自己小
		return
	}

	rf.Heartbeatchan <- true
	reply.Term = args.Term

	if rf.CurrentTerm < args.Term {
		rf.CurrentTerm = args.Term
		rf.State = FOLLOWER
		rf.VotedFor = -1
	}

	if rf.Entry[len(rf.Entry)-1].Index < args.PrevLogIndex {
		reply.NextLogIndex = rf.Entry[len(rf.Entry)-1].Index + 1
		return
	}

	prevlogterm := rf.Entry[args.PrevLogIndex].Term
	if prevlogterm != args.PrevLogTerm {         // 索引值相同但是任期号不同，不一致
		for i := args.PrevLogIndex-1 ; i >= 0 ; i-- {   // 寻找上一个任期的索引，作为 NextLogIndex返回
			if rf.Entry[i].Term != prevlogterm {
				reply.NextLogIndex = i + 1
				return
			}
		}
	} else {                                     // 满足一致性，附加日志
		reply.Success = true
		rf.Entry = rf.Entry[:args.PrevLogIndex+1]
		rf.Entry = append(rf.Entry,args.Entries...)
	}

	// 只有成功写数据才会到这一步
	// commitIndex 等于 leaderCommit 和 新日志条目索引值中较小的一个
	// commitIndex 是 可以被应用到状态机上的最大索引，也就是被提交的日志条目的最大索引
	if args.LeaderCommit > rf.CommitIndex {
		if args.LeaderCommit > rf.Entry[len(rf.Entry)-1].Index {
			rf.CommitIndex = rf.Entry[len(rf.Entry)-1].Index
		} else {
			rf.CommitIndex = args.LeaderCommit
		}
		rf.CommitLogchan <- true
	}
}

func (rf *Raft) sendAppendEntries(server int, args AppendEntriesArgs, reply *AppendEntriesReply) bool {
	ok := rf.peers[server].Call("Raft.AppendEntries", args, reply)
	rf.mu.Lock()
	defer rf.mu.Unlock()
	if ok {
		if rf.State != LEADER {                                 // 不再是 leader
			return ok
		}

		if args.Term != rf.CurrentTerm {
			return ok
		}

		if reply.Term > rf.CurrentTerm {                        // 遇到自己大的任期值，变为follwer
			rf.CurrentTerm = reply.Term
			rf.State = FOLLOWER
			rf.VotedFor = -1
			rf.persist()
			return ok
		}
		if reply.Success {
			if len(args.Entries) > 0 {                              // 为0就是heartbeat，不用修改NextIndex和MatchIndex
				rf.NextIndex[server] = args.Entries[len(args.Entries)-1].Index + 1 // 下一次要发给他的日志条目索引
				//fmt.Printf("Success %v %v len %v\n", server, rf.NextIndex[server], len(args.Entries))
				rf.MatchIndex[server] = rf.NextIndex[server] - 1 // 已经复制给他的日志的最高索引值
			}
		} else {
			rf.NextIndex[server] = reply.NextLogIndex
		}
	}
	return ok
}

func (rf *Raft) broadcastAppendEntries() {
	rf.mu.Lock()
	defer rf.mu.Unlock()

	Index := rf.CommitIndex
	for i := rf.CommitIndex + 1 ; i <= rf.Entry[len(rf.Entry)-1].Index ; i++ {
		Num := 1
		for index := range rf.peers {
			if index != rf.me && rf.State == LEADER && rf.MatchIndex[index] >= i && rf.Entry[i].Term == rf.CurrentTerm {
				Num++
			}
		}
		if 2*Num > len(rf.peers) {
			Index = i
		}
	}

	if Index != rf.CommitIndex {
		rf.CommitIndex = Index
		rf.CommitLogchan <- true
	}

	for index := range rf.peers {
		if index != rf.me && rf.State == LEADER {
			//fmt.Printf("%v heartbeat %v",rf.me,index)
			args := AppendEntriesArgs{}
			args.LeadId = rf.me
			args.Term = rf.CurrentTerm
			args.LeaderCommit = rf.CommitIndex
			args.PrevLogIndex = rf.NextIndex[index] - 1                            //  新日志的前一条的索引
			args.PrevLogTerm = rf.Entry[args.PrevLogIndex].Term          //  新日志的前一条的任期值
			args.Entries = append(args.Entries,rf.Entry[args.PrevLogIndex+1:]...)
			go func(i int, args AppendEntriesArgs) {
				reply := AppendEntriesReply{}
				rf.sendAppendEntries(i,args,&reply)
			} (index, args)
		}
	}

}


//
// the service using Raft (e.g. a k/v server) wants to start
// agreement on the next command to be appended to Raft's log. if this
// server isn't the leader, returns false. otherwise start the
// agreement and return immediately. there is no guarantee that this
// command will ever be committed to the Raft log, since the leader
// may fail or lose an election.
//
// the first return value is the index that the command will appear at
// if it's ever committed. the second return value is the current
// term. the third return value is true if this server believes it is
// the leader.
//
func (rf *Raft) Start(command interface{}) (int, int, bool) {
	rf.mu.Lock()
	defer rf.mu.Unlock()
	index := -1
	term := rf.CurrentTerm
	isLeader := rf.State == LEADER
	if isLeader {
		index = rf.Entry[len(rf.Entry)-1].Index + 1
		rf.Entry = append(rf.Entry,Log{Term:term,Command:command,Index:index})
		rf.persist()
	}
	return index, term, isLeader
}

//
// the tester calls Kill() when a Raft instance won't
// be needed again. you are not required to do anything
// in Kill(), but it might be convenient to (for example)
// turn off debug output from this instance.
//
func (rf *Raft) Kill() {
	// Your code here, if desired.
}

//
// the service or tester wants to create a Raft server. the ports
// of all the Raft servers (including this one) are in peers[]. this
// server's port is peers[me]. all the servers' peers[] arrays
// have the same order. persister is a place for this server to
// save its persistent state, and also initially holds the most
// recent saved state, if any. applyCh is a channel on which the
// tester or service expects Raft to send ApplyMsg messages.
// Make() must return quickly, so it should start goroutines
// for any long-running work.
//
func Make(peers []*labrpc.ClientEnd, me int,
	persister *Persister, applyCh chan ApplyMsg) *Raft {
	rf := &Raft{}
	rf.peers = peers
	rf.persister = persister
	rf.me = me

	// Your initialization code here.
	rf.State = FOLLOWER
	rf.VotedFor = -1
	rf.CurrentTerm = 0
	rf.CommitIndex = 0
	rf.VoteCount = 0
	rf.LastApplied = 0
	rf.Heartbeatchan = make(chan bool,100)
	rf.IsLeaderchan = make(chan bool,100)
	rf.VotedForchan = make(chan bool,100)
	rf.GrantVotechan = make(chan bool,100)
	rf.CommitLogchan = make(chan bool,100)
	rf.Entry = append(rf.Entry,Log{Term:0,Index:0})

	rf.readPersist(persister.ReadRaftState())

	go func() {
		for {
			switch rf.State {
			case FOLLOWER:
				select {
				case <-rf.GrantVotechan: // 为别人投票
				case <-rf.Heartbeatchan: // 收到heartbeat
				case <-time.After(time.Duration(rand.Int63()%300+ 500) * time.Millisecond): // 超时
					rf.mu.Lock()
					rf.persist()
					rf.State = CANDIDATE // 超时，变为candidate
					rf.mu.Unlock()
				}
			case LEADER:
				go rf.broadcastAppendEntries()
				time.Sleep(40*time.Millisecond)
			case CANDIDATE:
				rf.mu.Lock()
				rf.CurrentTerm++
				rf.VotedFor = rf.me
				rf.VoteCount = 1
				rf.persist()
				rf.mu.Unlock()
				// fmt.Printf("%v become candidate Term %v\n",rf.me,rf.CurrentTerm)
				go rf.broadcastRequestVote()
				select {
				case <-time.After(time.Duration(rand.Int63()%300+ 500) * time.Millisecond):
				case <-rf.Heartbeatchan:
					rf.mu.Lock()
					rf.persist()
					rf.State = FOLLOWER
					rf.mu.Unlock()
				case <-rf.IsLeaderchan:
					//fmt.Printf("%v become leader! Term %v\n",rf.me,rf.CurrentTerm)
					rf.mu.Lock()
					rf.persist()
					rf.State = LEADER // 赢得选举变为leader
					rf.NextIndex = make([]int, len(rf.peers))
					rf.MatchIndex = make([]int, len(rf.peers))
					for index := range rf.peers {
						rf.NextIndex[index] = rf.Entry[len(rf.Entry)-1].Index + 1
						// 使用的时候要减去一
						rf.MatchIndex[index] = 0
					}
					rf.mu.Unlock()
				}
			}
		}
	} ()


	// 用于收集appendenrtiese之后从channel返回的commit信息
	go func() {
		for {
			select {
			case <-rf.CommitLogchan:                       // 有成功附加日志
				rf.mu.Lock()
				for i := rf.LastApplied + 1; i <= rf.CommitIndex ; i++ {
					msg := ApplyMsg{}
					msg.Command = rf.Entry[i].Command
					msg.Index = i
					applyCh<-msg
				}
				rf.LastApplied = rf.CommitIndex
				rf.mu.Unlock()
			}
		}
	} ()

	return rf
}


伪代码：

MakeRaft() {
	// 初始化 Raft
	initRaft()

	开一个 goroutinue
}
