#! /bin/bash

if [ ! -d deps/leveldb ]; then
    echo "cloning leveldb from google code"
    (mkdir -p deps && cd deps && git clone https://code.google.com/p/leveldb/ && cd leveldb && make -j)
    sudo cp -r deps/leveldb/include/leveldb /usr/local/include/
    case $OSTYPE in
        linux-gnu)
            sudo cp deps/leveldb/libleveldb.so.1.15 /usr/lib
            sudo ln -s /usr/lib/libleveldb.so.1.15 /usr/lib/libleveldb.so
            sudo ln -s /usr/lib/libleveldb.so.1.15 /usr/lib/libleveldb.so.1
            ;;
        *)                      #  os x
            cp deps/leveldb/libleveldb.dylib.1.15 /usr/lib
            sudo ln -s /usr/lib/libleveldb.dylib.1.15 /usr/lib/libleveldb.dylib
            ;;
    esac

else
    echo "update leveldb from google code"
    (cd deps/leveldb && git pull && make -j)
fi

go get github.com/jmhodges/levigo

# CGO_CFLAGS="-I/home/feng/workspace/proxies/deps/leveldb/include" CGO_LDFLAGS="-L/home/feng/workspace/proxies/deps/leveldb" go get github.com/jmhodges/levigo

# CGO_CFLAGS="-I/home/feng/workspace/proxies/deps/leveldb/include" CGO_LDFLAGS="-L/home/feng/workspace/proxies/deps/leveldb/libleveldb.so.1.15" go get github.com/jmhodges/levigo

# ln -s ~/workspace/proxies/deps/leveldb/libleveldb.so.1.15 libleveldb.so.1
