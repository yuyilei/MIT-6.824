package mapreduce

import (
	"os"
	"fmt"
	"encoding/json"
	"sort"
)

// doReduce does the job of a reduce worker: it reads the intermediate
// key/value pairs (produced by the map phase) for this task, sorts the
// intermediate key/value pairs by key, calls the user-defined reduce function
// (reduceF) for each key, and writes the output to disk.

// doReduce做 reduce的工作，它读入中间产生的 key/value对(map phase产生的)
// 根据 key 排序 key/value ，对每个key调用用户定义的reduce函数，把输出写到磁盘上。
func doReduce(
	jobName string, // the name of the whole MapReduce job
	reduceTaskNumber int, // which reduce task this is
	nMap int, // the number of map tasks that were run ("M" in the paper)   // map task的数量
	reduceF func(key string, values []string) string,
) {
	// TODO:
	// You will need to write this function.
	// You can find the intermediate file for this reduce task from map task number
	// m using reduceName(jobName, m, reduceTaskNumber).
	// Remember that you've encoded the values in the intermediate files, so you
	// will need to decode them. If you chose to use JSON, you can read out
	// multiple decoded values by creating a decoder, and then repeatedly calling
	// .Decode() on it until Decode() returns an error.
	//
	// 你能通过map task number找到这个reduce task的的中间文件(通过reduceName函数)
	// 你encode了这些在中间文件中的数据，所以你需要去decode这些数据。
	// 如果你选择了json，你可以通过创造一个decoder去decode这些数据，然后调用Decode()函数，直到返回一个错误。
	//
	// You should write the reduced output in as JSON encoded KeyValue
	// objects to a file named mergeName(jobName, reduceTaskNumber). We require
	// you to use JSON here because that is what the merger than combines the
	// output from all the reduce tasks expects. There is nothing "special" about
	// JSON -- it is just the marshalling format we chose to use. It will look
	// something like this:
	//
	// 你应该把reduced的输出作为一个json encoded 的键值对象存储到一个文件中，文件名由mergeName得到
	// 在这，你需要使用Json.
	//
	// enc := json.NewEncoder(mergeFile)
	// for key in ... {
	// 	enc.Encode(KeyValue{key, reduceF(...)})
	// }
	// file.Close()
	//
	//
	// 步骤：
	// 1. 打开map task产生的文件（reduceName得到reduce）
	// 2. decode这些文件（文件为json格式），得到数据，进行合并
	// 3. 产生文件，文件名从 mergeName得到
	// 4. 对合并之后的结果调用reduceF函数，将结果写入文件（写入顺序要根据 key进行排序）。
	//

	keyvalue := make(map[string][]string)
	// 存储所有的keyvalue
	for i := 0 ; i < nMap ; i++ {
		filename := reduceName(jobName,i,reduceTaskNumber)
		file, err := os.Open(filename)
		if err != nil {
			fmt.Print(err)
			fmt.Printf("打开文件 %s 失败!\n",filename)
			continue
			// 打开这个文件失败，还是继续尝试下个文件
		}
		fjson := json.NewDecoder(file)
		for {
			var kv KeyValue
			err_ := fjson.Decode(&kv)
			if err_ != nil {
				break
				// 数据全部decode了，返回一个错误
			}
			if _, ok := keyvalue[kv.Key]; ok {
				// key 已经存在
				keyvalue[kv.Key] = append(keyvalue[kv.Key], kv.Value)
			} else {
				// key 还不存在
				newvalue := []string{kv.Value}
				keyvalue[kv.Key] = newvalue

			}
		}
		file.Close()
	}

	file, err := os.Create(mergeName(jobName,reduceTaskNumber))
	defer file.Close()
	if err != nil {
		fmt.Print(err)
		fmt.Printf("打开merge文件 %s 失败!\n",mergeName(jobName,reduceTaskNumber))
		return
	}

	keys := make([]string,0,len(keyvalue))
	for k, _ := range keyvalue {
		keys = append(keys,k)
	}
	sort.Strings(keys)
	//递增

	enc := json.NewEncoder(file)
	for _, k := range keys {
		enc.Encode(KeyValue{k,reduceF(k,keyvalue[k])})
	}

}
