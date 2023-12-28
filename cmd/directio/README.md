# DirectIO Test

A simple tool for testing the performance of DirectIO (using "github.com/ncw/directio") by randomly sampling from an existing data file.

## Usage

```sh
Usage: direct [options]
  -d    Use directIO (default false)
  -f string
        File to test
  -s int
        Number of seconds to run (default 12)
  -t int
        Number of threads (default runtime.NumCPU())
```

### Examples

```sh
# Start random sampling from file test.dat with 12 threads for 12 seconds long, NOT using DirectIO
./directio  -f="test.dat" -t=12
# Start random sampling from file test.dat with 12 threads for 12 seconds long, using DirectIO
./directio  -f="test.dat" -t=12 -d
```