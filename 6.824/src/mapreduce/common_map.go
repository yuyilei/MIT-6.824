package mapreduce

import (
	"hash/fnv"
	"os"
	"fmt"
	"encoding/json"
)

// doMap does the job of a map worker: it reads one of the input files
// (inFile), calls the user-defined map function (mapF) for that file's
// contents, and partitions the output into nReduce intermediate files.
// doMap 做 map worker 的工作，它读入一个输入文件，对文件的内容调用用
// 户定义的map函数，将输出分配到n个 Reduce 中间文件中。
func doMap(
	jobName string, // the name of the MapReduce job
	mapTaskNumber int, // which map task this is
	inFile string,  // 输入文件的文件名
	nReduce int, // the number of reduce task that will be run ("R" in the paper)
	mapF func(file string, contents string) []KeyValue, // 用户定义的map函数
) {
	// TODO:
	// You will need to write this function.
	// You can find the filename for this map task's input to reduce task number
	// r using reduceName(jobName, mapTaskNumber, r). The ihash function (given
	// below doMap) should be used to decide which file a given key belongs into.
	//
    // 通过reduceName你能找到文件名，作为map task的输入，并给reduce task number.
    // ihash函数决定每个key属于的文件. 应该是把key给哈希了
    // 使用 reduceName(jobName, mapTaskNumber, r) , r是reduce task number产生 reduceName? 好像是这个意思
    //
	// The intermediate output of a map task is stored in the file
	// system as multiple files whose name indicates which map task produced
	// them, as well as which reduce task they are for. Coming up with a
	// scheme for how to store the key/value pairs on disk can be tricky,
	// especially when taking into account that both keys and values could
	// contain newlines, quotes, and any other character you can think of.
	//
    // 一个map task的中间输入作为 multiple files 被储存在文件系统上，它们的名字应该要
    // 显示哪个map task产生了它们和它们是属于哪个reduce task的，想出如何在磁盘上储存
    // key/value是棘手的，尤其是考虑到key和value都包含新行，引用，和任何你能想到的字符。
    //
	// One format often used for serializing data to a byte stream that the
	// other end can correctly reconstruct is JSON. You are not required to
	// use JSON, but as the output of the reduce tasks *must* be JSON,
	// familiarizing yourself with it here may prove useful. You can write
	// out a data structure as a JSON string to a file using the commented
	// code below. The corresponding decoding functions can be found in
	// common_reduce.go.
	//
    // json经常被用于序列化数据到字节流。你不一定需要使用json，但是reduce task的输出一定是
    // json，你可以将写一个数据结构作为json字符串输出到文件（使用如下代码），对应的解码函数在
    // common_reduce中
    //
	//   enc := json.NewEncoder(file)
	//   for _, kv := ... {
	//     err := enc.Encode(&kv)
	//
	// Remember to close the file after you have written all the values!
	// 当你写入所有值之后记得要关闭文件！


	// 大致流程：
	// 1. 打开文件，读出全部数据
	// 2. 调用用户定义的mapF函数，得到 key-value的切片 kv
	// 3. kv中的数据根据key分类(hash得到索引)，创建一系列中间文件（使用reduceName函数得到文件名），文件为json格式
	// 4. 根据（key产生的）索引，将key-value存到不同的文件上


	f, err := os.Open(inFile)
	defer f.Close()
	if err != nil {
		fmt.Print(err)
		fmt.Println("文件打开错误!")
		return
	}

	finfo, err := f.Stat()
	if err != nil {
		fmt.Print(err)
		fmt.Println("获取文件信息失败!")
		return
	}

	fcontent := make([]byte,finfo.Size())
	// fcontent现在是byte切片，之后要被转化为string
	f.Read(fcontent)
	// 从文件中读出字节流
	contents := string(fcontent)
	// 转化为string

	keyvalue := mapF(inFile,contents)
	// 调用用户定义的函数，keyvalue 是键值对的切片
	fjson := make([]*json.Encoder,nReduce)
	files := make([]*os.File,nReduce)
	// json 文件格式的切片

	// 创建一系列文件
	for i := range(fjson) {
		filename := reduceName(jobName,mapTaskNumber,i)
		// 得到文件名
		file, err := os.Create(filename)
		if err != nil {
			fmt.Print(err)
			fmt.Printf("创建文件 %s 失败!",filename)
			return
		}
		fjson[i] = json.NewEncoder(file)
		files[i] = file
		// 此时不能关闭文件，如果这时候关闭了文件，后续就不能写入了
	}

	// 分类存入文件
	for _, kv := range keyvalue {
		index := ihash(kv.Key) % uint32(nReduce)
		// 索引，决定存储的位置
		fjson[index].Encode(&kv)
		// 相应位置存入 json数据
	}

	// 最后关闭文件
	for _, file := range files {
		file.Close()
	}

}

func ihash(s string) uint32 {
	h := fnv.New32a()
	h.Write([]byte(s))
	return h.Sum32()
}
