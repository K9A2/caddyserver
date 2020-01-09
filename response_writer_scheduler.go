package caddy

import (
	"io/ioutil"
	"bytes"
    "net/http"
	"fmt"
	// "fmt"
    "os"
    "sync"
    "time"

    // "github.com/google/logger"
)

// youtube 的 managed 请求
var youtubeList = []string{
    // "index.html",
    // "www-onepick-2x-webp-vflsYL2Tr.css",
    // "YTSans300500700.css",
    // "RobotoMono400700.css",
    // "www-main-desktop-watch-page-skeleton-2x-webp-vflQ9GNSj.css",
    // "www-main-desktop-home-page-skeleton-2x-webp-vfl4iT_wE.css",
    // "Roboto400300300italic400italic500500italic700700italic.css",
    // "www-prepopulator.js",
    // "custom-elements-es5-adapter.js",
    // "scheduler.js",
    // "www-tampering.js",
    // "network.js",
    // "spf.js",
    // "web-animations-next-lite.min.js",
    // "webcomponents-sd.js",
    // "desktop_polymer_v2.js",
    // "KFOmCnqEu92Fr1Mu4mxK.woff2",
    // "KFOlCnqEu92Fr1MmEU9fBBc4.woff2",
    "index.html",
    "desktop_polymer_v2.js",
    "web-animations-next-lite.min.js",
    "webcomponents-sd.js",
    "scheduler.js",
    "custom-elements-es5-adapter.js",
    "spf.js",
    "desktop_polymer_v2-vflfD_pIA.html",
    "www-onepick-2x-webp-vflsYL2Tr.css",
    "YTSans300500700.css",
    "RobotoMono400700.css",
    "www-main-desktop-watch-page-skeleton-2x-webp-vflQ9GNSj.css",
    "www-main-desktop-home-page-skeleton-2x-webp-vfl4iT_wE.css",
    "Roboto400300300italic400italic500500italic700700italic.css",
    "www-player-2x-webp.css",
    "www-prepopulator.js",
    "www-tampering.js",
    "network.js",
    "KFOmCnqEu92Fr1Mu4mxK.woff2",
    "KFOlCnqEu92Fr1MmEU9fBBc4.woff2",
}

var fileCache = make(map[string]*bytes.Reader) 

func initFileCache() {
    for _, filename := range youtubeList {
        content, err := ioutil.ReadFile(filename) 
        if err != nil {
            fmt.Printf("error in loading cache file <%v>, err <%v>\n", filename, err.Error())
            return
        }
        fmt.Printf("cached file <%v>\n", filename)
        fileCache[filename] = bytes.NewReader([]byte(content))
    }
}

const criticalResource = "desktop_polymer_v2.js"

// 能同时 copy 数据的 ResponseWriter 最大数目
const maxConcurrentResponseWriter = 4

// 有新的 response writer 到达时向此 chan 发送消息
var blockArriveChan = make(chan *responseWriterControlBlock, 20)

// 现有的 response writer 执行完毕时向此 chan 发送消息
var blockFinishChan = make(chan *responseWriterControlBlock, 20)

// MoveOnChan 使得 FileServer 的 ServeHTTP 在从此 chan 收到发送给自己的信号之后才会退出
var MoveOnChan = make(chan string, 20)

// ResponseWriter 调度控制块定义
type responseWriterControlBlock struct {
    writer   http.ResponseWriter
    request  *http.Request
    name     string
    modtime  time.Time
    filename string
}

// 用给定信息创建一个新的 ResponseWriter 调度控制块
func newResponseWriterControlBlock(w http.ResponseWriter, r *http.Request,
    n string, m time.Time, f string) *responseWriterControlBlock {
    return &responseWriterControlBlock{
        writer:   w,
        request:  r,
        name:     n,
        modtime:  m,
        filename: f,
    }
}

// ResponseWriterScheduler 是用来调度 response writer 的调度器
type ResponseWriterScheduler struct {
    mux sync.Mutex

    ManagedResponseWriter            []*responseWriterControlBlock
    ManagedURLMap                    map[string]int
    ActiveManagedResponseWriterCount int

    UnmanagedResponseWriter            []*responseWriterControlBlock
    ActiveUnmanagedResponseWriterCount int
    // 正在执行的 ResponseWriter 数目
    concurrentResponseWriterCount int

    // transmittingCriticalResource bool
}

// 调度器实例变量
var responseWriterScheduler ResponseWriterScheduler

// initResponseWriterScheduler 初始化一个新的调度器实例
func initResponseWriterScheduler() {
    responseWriterScheduler = ResponseWriterScheduler{
        ManagedResponseWriter:        make([]*responseWriterControlBlock, len(youtubeList)),
        ManagedURLMap:                make(map[string]int),
        UnmanagedResponseWriter:      make([]*responseWriterControlBlock, 0),
        // transmittingCriticalResource: false,
    }
    // 添加已经安排好传输顺序的 url 和对应次序到 map 中
    for index, url := range youtubeList {
        responseWriterScheduler.ManagedURLMap[url] = index
    }
}

// RunResponseWriterScheduler 在后台运行调度器
func RunResponseWriterScheduler() {
    initResponseWriterScheduler()
    // logger.Info("response scheduler inited and running")
    // 通过两个不同的 chan 来接受外部命令
    for {
        // if !responseWriterScheduler.transmittingCriticalResource {
        responseWriterScheduler.tryRunNextResponseWriter()
        // }
        select {
        case newBlock := <-blockArriveChan:
            responseWriterScheduler.onResponseWriterArrive(newBlock)
        case finishedBlock := <-blockFinishChan:
            responseWriterScheduler.onResponseWriterFinished(finishedBlock)
        }
    }
}

// AddNewResponseWriterControlBlock 在 caddyserver 有新的请求到来时被用来向调度器
// 添加一个新的 ResponseWriter 控制块，供调度器在适当时机执行对应的 ResponseWriter
func AddNewResponseWriterControlBlock(w http.ResponseWriter, req *http.Request,
    name string, modtime time.Time, file *os.File) {
    newBlock := newResponseWriterControlBlock(w, req, name, modtime, file.Name())
    // responseWriterScheduler.onResponseWriterArrive(newBlock)
    blockArriveChan <- newBlock
}

// findFirstAvailableResponseWriter 返回找到的第一个非空的 block，找不到时返回 nil
func findFirstAvailableResponseWriter(
    queue *[]*responseWriterControlBlock) *responseWriterControlBlock {
    for index, val := range *queue {
        if val != nil {
            (*queue)[index] = nil
            return val
        }
    }
    return nil
}

// addNewBlock 向调度器添加一个新的 block
func (schd *ResponseWriterScheduler) addNewBlock(block *responseWriterControlBlock) {
    index, ok := responseWriterScheduler.ManagedURLMap[block.filename]
    if ok {
        // 此 block 为 managed 的 stream 所有
        schd.ManagedResponseWriter[index] = block
        // logger.Infof("add managed block with url <%v> to pos <%v>",
        // 	block.filename, index)
        return
    }
    // 把 unmanaged 的 stream 添加到轮询队列中
    schd.UnmanagedResponseWriter = append(schd.UnmanagedResponseWriter, block)
    // logger.Infof("add unmanaged block with url <%v>", block.filename)
}

// popNextResponseWriter 提供下一个可以执行的 ResponseWriter，并从队列中移除对应的 block
// 如果找不到可以执行的 ResponseWriter，则此函数返回 nil。
func (schd *ResponseWriterScheduler) popNextResponseWriter() *responseWriterControlBlock {
    var next *responseWriterControlBlock
    // 可以被调度器调度执行的 ResponseWriter 数目有最大限制
    if schd.concurrentResponseWriterCount < maxConcurrentResponseWriter {
        next = findFirstAvailableResponseWriter(&schd.ManagedResponseWriter)
        if next != nil {
            // 执行此 ResponseWriter
            schd.ActiveManagedResponseWriterCount++
            // logger.Infof("popNextResponseWriter: unmanged count <%v>, url <%v>",
            //     len(schd.UnmanagedResponseWriter), next.filename)
            // for _, val := range schd.ManagedResponseWriter {
            //     if val != nil {
            //         logger.Infof("  filename <%v>", val.filename)
            //     }
            // }
            // fmt.Printf("pop managed resource <%v>\n", next.filename)
            goto moveOn
        }
        if schd.ActiveManagedResponseWriterCount > 0 || schd.ActiveUnmanagedResponseWriterCount > 0 {
            // 在有 managed stream 传输时不进行 unmanaged stream 的传输
            // 也不会有超过一个 unmanaged 的 Response Writer 在传输
            goto moveOn
        }
        if len(schd.UnmanagedResponseWriter) > 0 {
            // unmanaged 队列中有可用的 ResponseWriter
            next = schd.UnmanagedResponseWriter[0]
            schd.UnmanagedResponseWriter = schd.UnmanagedResponseWriter[1:]
            schd.ActiveUnmanagedResponseWriterCount++
            // logger.Infof("popNextResponseWriter: unmanged count <%v>, url <%v>",
            //     len(schd.UnmanagedResponseWriter), next.filename)
            // for _, val := range schd.ManagedResponseWriter {
            //     if val != nil {
            //         logger.Infof("  filename <%v>", val.filename)
            //     }
            // }
            // fmt.Printf("pop unmanaged resource <%v>\n", next.filename)
            goto moveOn
        }
    }

moveOn:
    return next
}

func (schd *ResponseWriterScheduler) tryRunNextResponseWriter() {
    schd.mux.Lock()
    // logger.Info("tryRun lock")
    available := schd.popNextResponseWriter()
    if available != nil {
        // if available.filename == criticalResource {
        //     schd.transmittingCriticalResource = true
        //     fmt.Println("transmitting critical resource")
        // }
        schd.concurrentResponseWriterCount++
        go schd.executeResponseWriter(available)
        // logger.Infof("concurrent response writer <%v>, url <%v>", schd.concurrentResponseWriterCount, available.filename)
    }
    schd.mux.Unlock()
    // logger.Info("tryRun unlock")
}

// onResponseWriterArrive 向调度器添加一个新的待调度的 ResponseWriter
func (schd *ResponseWriterScheduler) onResponseWriterArrive(
    block *responseWriterControlBlock) {
    schd.mux.Lock()
    // 把新来的 block 添加到合适的队列中
    schd.addNewBlock(block)
    schd.mux.Unlock()
}

// executeResponseWriter 负责实际执行 block 所指定的 ResponseWriter
func (schd *ResponseWriterScheduler) executeResponseWriter(
    block *responseWriterControlBlock) {
    cachedFile, ok := fileCache[block.filename]
    if ok {
        http.ServeContent(block.writer, block.request, block.name, block.modtime, cachedFile)
        goto moveOn
    } else {
        file, err := os.Open(block.filename)
        if err != nil {
            // logger.Infof("error in opening file, <%v>", err.Error())
            return
        }
        // logger.Infof("executing url <%v>", block.filename)
        http.ServeContent(block.writer, block.request, block.name, block.modtime, file)
        file.Close()
    }

moveOn:
    // 让被阻塞的函数继续执行
    MoveOnChan <- block.filename
    // logger.Infof("response writer <%v> finished", block.filename)
    // fmt.Printf("resource finish <%v>\n", block.filename)
    blockFinishChan <- block
    // schd.onResponseWriterFinished(block)
}

// remove 从队列中移除指定元素
func remove(
    queue *[]*responseWriterControlBlock,
    block *responseWriterControlBlock) {
    for index, val := range *queue {
        if val.request.RequestURI == block.request.RequestURI {
            // 从 index 出断开
            *queue = append((*queue)[:index], (*queue)[:index+1]...)
            // logger.Infof("remove url <%v> from unmanaged queue", block.request.RequestURI)
            return
        }
    }
}

// OnResponseWriterFinished 在有 ResponseWriter 写完所有内容时被触发
func (schd *ResponseWriterScheduler) onResponseWriterFinished(
    block *responseWriterControlBlock) {
    defer schd.mux.Unlock()
    schd.mux.Lock()

    // if block.filename == criticalResource {
    //     schd.transmittingCriticalResource = false
    //     fmt.Println("critical resource finished")
    // }

    schd.concurrentResponseWriterCount--
    _, ok := schd.ManagedURLMap[block.filename]
    if ok {
        schd.ActiveManagedResponseWriterCount--
        return
    }
    schd.ActiveUnmanagedResponseWriterCount--
}
