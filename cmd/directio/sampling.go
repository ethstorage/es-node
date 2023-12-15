package main

import (
	"context"
	"flag"
	"fmt"
	"math/rand"
	"os"
	"runtime"
	"sync"
	"sync/atomic"
	"time"

	"github.com/ncw/directio"
)

func sample(numThreads int, duration time.Duration, filename string, direct bool) {
	var err error
	var file *os.File
	if direct {
		file, err = directio.OpenFile(filename, os.O_RDONLY, 0666)
	} else {
		file, err = os.OpenFile(filename, os.O_RDONLY, 0666)
	}
	if err != nil {
		panic(err)
	}
	defer file.Close()

	info, _ := file.Stat()
	fileSize := info.Size()
	fmt.Println("File size:", fileSize)
	ctx, cancel := context.WithTimeout(context.Background(), duration)
	defer cancel()

	var counter int32
	var wg sync.WaitGroup
	for i := 0; i < numThreads; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()

			for {
				select {
				case <-ctx.Done():
					return
				default:
					var data []byte
					if direct {
						data = directio.AlignedBlock(directio.BlockSize)
					} else {
						data = make([]byte, 4096)
					}
					offset := rand.Int63n(fileSize)
					offset = offset / 32 * 32
					_, err := file.ReadAt(data, offset)
					if err != nil {
						fmt.Println(err)
						return
					}
					atomic.AddInt32(&counter, 1)
				}
			}
		}()
	}
	wg.Wait()
	fmt.Printf("Total sampling times: %v\n", atomic.LoadInt32(&counter))
}

func main() {
	var (
		filename string
		seconds  int
		threads  int
		dio      bool
	)
	flag.StringVar(&filename, "f", "", "File to do sampling")
	flag.IntVar(&threads, "t", runtime.NumCPU(), "Number of threads")
	flag.IntVar(&seconds, "s", 12, "Number of seconds to run")
	flag.BoolVar(&dio, "d", false, "Use directIO (default false)")
	flag.Usage = func() {
		fmt.Println("Usage: direct [options]")
		flag.PrintDefaults()
	}

	flag.Parse()
	duration := time.Duration(seconds) * time.Second
	var d string
	if !dio {
		d = " NOT"
	}

	fmt.Printf("Start random sampling from file %s with %d threads for %d seconds long,%s using DirectIO\n",
		filename,
		threads,
		seconds,
		d,
	)
	sample(threads, duration, filename, dio)
}
