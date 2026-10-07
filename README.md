# Kairo

Kairo is a small cache I wrote in Go. You put a string in, give it a name, and optionally a time to live. Later you ask for that name and get the string back. When the time runs out, or when the process stops, the string is gone.

It speaks RESP, which is the protocol Redis already uses. So `redis-cli` works, and so do the Redis clients people already have in Go, Python, Node, Java, and so on. You point them at a different port. The commands they send for this small set stay the same.

I built it because a lot of Redis setups are not really using Redis. The app saves a string, sets a TTL, and reads it back. Sessions, bits of a page, a database row that is fine to load again. Redis can do that, and it also does persistence, scripting, pub/sub, lists, sets, hashes, all on one thread. I wanted to see what happens if you throw the extra stuff out and spend the other cores on the cache path.

It is faster than Redis when a bunch of clients talk to it at once. It uses more RAM per key. A restart wipes it. If those three things are acceptable, it is useful. If any of them is a problem, it is the wrong tool.

There is no password, no `INCR`, no hashes, no lists, no replication, and nothing is written to disk.

## The machine behind the numbers

I ran this on my laptop, not a server. Linux, 12 CPUs, Intel Core i7-1355U, Go 1.27, Redis 8.10.1. Same client for both. 50,000 keys, 64-byte values, every `SET` with a 600 second expiry. Redis had saving turned off, so this is cache against cache.

| What I tried | Kairo | Redis | Would I use Kairo? |
| --- | --- | --- | --- |
| 32 clients, 16 commands at a time | 884,296 / sec, slow batch 2.3 ms | 327,113 / sec, slow batch 3.8 ms | Yes. This is how apps actually talk to a cache |
| 32 clients, one command at a time | 130,015 / sec, slow call about 1.0 ms | 56,512 / sec, about 1.7 ms | Yes, if one Redis process is stuck on one core and the data fits |
| 1 client | 13,936 / sec on this run | 11,731 / sec on this run | No. Other runs on the same laptop had Redis ahead. I would not quote this row |
| RAM after loading the keys | about 325 bytes per key | about 160 bytes per key | No, if you are paying for RAM and you have a huge number of tiny keys |
| Restart the process | empty | still there, if you turned saving on | No. Use Redis or Valkey for that |

Ten million small keys would be roughly 3.3 GB here and 1.6 GB in Redis, before the values get big. The speed is worth it when the data fits and the app is waiting on Redis's one thread. It is a bad trade when the bill is memory.

## Where I would actually put it

The app still owns the real data. Kairo holds a copy that is allowed to disappear. If a restart would lose something you cannot rebuild, do not put that something here.

### In front of a database

This is the one I had in mind the whole time.

Say a page loads the same user, product, or cart row over and over. The database does the same query for every request. The app can keep the answer in Kairo for a minute:

1. `GET user:42`
2. If something comes back, use it.
3. If the reply is empty, read the database.
4. `SET user:42 '{"name":"Ada"}' EX 60`
5. When that user is updated in the database, `DEL user:42`. Otherwise everyone keeps seeing the old copy until the minute is up.

A page that needs a few of these at once can send `MGET user:42 product:9 cart:42`. You get one slot per key, in the same order. Empty slots are the ones to load from the database and write back.

This lines up with the fast number above: lots of app servers, short strings, a TTL on the write, commands sent in a batch. On the laptop, 32 clients sending 16 at a time did about 884k commands a second.

Kairo does not watch the database. Forget the `DEL` and the cache is stale until the TTL ends. Restart Kairo and every read misses at once, so the database has to be able to take that spike.

```sh
redis-cli -p 8989 SET user:42 '{"name":"Ada"}' EX 60
redis-cli -p 8989 GET user:42
redis-cli -p 8989 TTL user:42
redis-cli -p 8989 DEL user:42
```

### Sessions, if login can happen again

After login:

```sh
redis-cli -p 8989 SET session:8f3a '{"user":42}' EX 1800
```

Each request does `GET session:8f3a`. Logout is `DEL`. If you want the 30 minutes to slide while they are still clicking around, `EXPIRE session:8f3a 1800`. `TTL` tells you how many seconds are left.

A restart logs everybody out. That is fine for some apps and a bad afternoon for others. If the session has to survive a deploy, keep it somewhere that writes to disk, and only use Kairo as a faster copy in front.

Keep it small. Anything over 1 MiB is refused.

### Something expensive that is fine if it is a few seconds old

A rendered bit of HTML, a slow internal call, a report nobody needs live. First request does the work and stores it:

```sh
redis-cli -p 8989 SET fragment:home:hero "<html>" EX 30
```

Everyone else for the next 30 seconds just `GET`s it. When it expires, the next request does the work again.

The annoying part is the moment it expires, or the moment Kairo restarts. Every server misses together and they all hit the origin. If that stampede is a problem, this pattern needs a lock or a single refresher in the app. Kairo will not do that for you.

### Remembering that you already handled something

A webhook delivery, a request id, a one-time token. You only need to know "I saw this" for a while:

```sh
redis-cli -p 8989 SET seen:webhook:91c2 1 EX 86400
```

Next time, `GET` it. If it is there, skip the work. If it is not, do the work and `SET` the key.

Only do this when running the work twice is safe, or when the database still has the real uniqueness check and Kairo is just skipping the common case. A restart forgets every "seen" key, and the retry will look new.

I did not add `INCR`. You cannot keep a rate-limit counter that goes 1, 2, 3. A yes/no flag with a TTL is as far as this goes.

### One cache for several app processes

Kairo is a process with a port. Every app server on the private network can connect to the same address and see the same keys. A map inside each app process would not do that. Each process would fill its own copy and miss the ones the others already fetched.

I would use Kairo when there is more than one app server, the values are small strings, the code already speaks Redis, and an empty cache after a restart is ok.

I would keep the cache inside the process (Ristretto, Otter, whatever the framework ships) when there is only one process and the network hop is wasted. I would use Redis or Valkey when the data has to be on disk, or when the app needs commands I did not build.

## How to run it

Go 1.27 or newer. `redis-cli` is enough to poke at it. An application only needs whatever Redis library it already uses.

From this directory:

```sh
go run ./cmd/kairo
```

It listens on `:8989` and prints `Kairo listening on :8989`. Then, in another terminal:

```sh
redis-cli -p 8989 PING
redis-cli -p 8989 SET user:1 ada EX 60
redis-cli -p 8989 GET user:1
redis-cli -p 8989 TTL user:1
redis-cli -p 8989 MGET user:1 user:2
redis-cli -p 8989 DEL user:1
```

`PING` comes back `PONG`. `GET` on a missing or expired key is an empty reply. `TTL` is `-2` if the key is missing or already dead, `-1` if it never expires, otherwise the seconds left, rounded up.

If the app already has a Redis URL, change the port from 6379 to 8989. `AUTH`, `INCR`, `HSET`, `LPUSH`, `PUBLISH` and the rest come back as an error. Stick to `PING`, `SET`, `GET`, `MGET`, `DEL`, `EXPIRE`, and `TTL`.

The read path in the app is the usual one:

```text
value = GET key
if value is empty:
    value = load from the database
    SET key value EX 60
use value
```

And on a write:

```text
save the row in the database
DEL key
```

If the client has several commands, send them together. Kairo writes each reply into a buffer and flushes when the connection has nothing else waiting. One command still gets its answer right away. The benchmark used batches of 16. That number is not a limit.

There is no username and no password. Do not open 8989 to the internet. Localhost, or a private network the app servers can reach. To listen only on this machine:

```sh
go run ./cmd/kairo -addr 127.0.0.1:8989
```

`-addr` defaults to `:8989`.

### Capping memory

With no flag, it grows until keys expire or the machine runs out of RAM. You can cap the sum of key lengths and value lengths:

```sh
go run ./cmd/kairo -addr 127.0.0.1:8989 -max-memory 268435456
```

That is 256 MiB of the bytes you stored, not of the process. A `SET` that would go past it deletes the key closest to expiring, then stores the new value. Keys with no expiry are left alone. If nothing can be removed, or the new key by itself is bigger than the cap, the `SET` returns `ERR maxmemory reached` and the old value stays. Deletes and smaller updates still work.

Go's map overhead is extra, so the process will be bigger than the number you pass. From the run above, figure about 325 bytes of process RAM per small key once the data is loaded.

`-max-shards` changes how many pieces the key space is split into. The default is 16.

Ctrl-C (`SIGINT` or `SIGTERM`) stops new connections, closes the ones still open, stops the expiry worker, and exits.

### Restart

Stop it, start it, the keys are gone. The next `GET` misses. An app that loads from the database on a miss will fill the cache again. An app that stored the only copy of a session, or the only record that a webhook was handled, will treat those as new.

There is no snapshot to copy and no replica to fail over to.

### Tests and the comparison

```sh
go test ./...
go run ./cmd/kairobench
go test -bench=. -benchmem ./internal/engine ./internal/storage
```

`go test ./...` covers the protocol, the server, expiry, the wheel, and the memory cap. `go test -race ./...` runs the same tests with the race detector. The RESP parser also has a fuzz target, `FuzzReadCommand`. GitHub Actions runs the tests, the race detector, and `go vet` on every push.

`cmd/kairobench` builds Kairo, starts it on `127.0.0.1:18989`, runs the workload, stops it, then runs the same thing against Redis on `127.0.0.1:16379`. I used those ports so it would not step on a Redis already on 6379 or a Kairo already on 8989. Flags are `-keys`, `-rounds`, `-clients`, `-value-size`, `-ttl`, `-warmup`, and `-duration`.

The tests that hit the public API live in `test/`. The benchmarks sit next to `internal/engine` and `internal/storage`, because Go will not pick up a package's benchmarks from anywhere else.

## What happens to a command

```
client
  RESP
    cmd/kairo            one goroutine per connection
      internal/protocol  read a command, write a reply
      internal/server    check the arguments
      internal/engine    GET, SET, DEL, EXPIRE, TTL, and the cleaner
      internal/storage   16 maps, each with a lock and a 60-slot wheel
```

A client connects. The accept loop hands the socket to a new goroutine and goes back to waiting. That goroutine reads one command, checks it, and calls the engine. The engine hashes the key, locks that shard, and reads or writes the map. The reply goes into a buffer. If the client already sent more commands, they get handled and the buffer is flushed once at the end.

I split it this way so the socket code does not know about wheel slots, and the expiry code does not know about TCP.

| Folder | What lives there |
| --- | --- |
| `cmd/kairo` | The process. Flags, listen, one goroutine per connection |
| `cmd/kairobench` | The comparison. Not part of the server |
| `internal/protocol` | Bytes in, command out, and the other way around. Rejects anything over 1 MiB |
| `internal/server` | Which command it is, and whether the argument count is right |
| `internal/engine` | The cache rules, and the goroutine that wakes up once a second to delete expired keys |
| `internal/storage` | The maps, the locks, the byte counter, the wheels |
| `test/` | Tests written from outside the packages |

## Why these numbers, specifically

I kept getting asked why 16, why 60, why port 8989. Here is the actual reason for each, including the ones that are just "it was a reasonable constant and I measured it."

**Port 8989.** Redis is usually on 6379. I wanted both running while I compared them, so Kairo defaults to 8989. Change it with `-addr`. The benchmark uses 18989 and 16379 for the same reason: don't measure whatever I happened to leave running.

**1 MiB.** That is 1,048,576 bytes. If a client asks for a bigger bulk string, the reader stops. A cache value that size is not what this is for, and I didn't want one connection to force a huge allocation. The check happens before anything is stored.

**One goroutine per connection.** Yes, Kairo is multi-threaded. The accept loop does not run the command. Each connection gets a goroutine, and Go runs those goroutines on several operating-system threads, so more than one core can be busy at once. A separate goroutine wakes up once a second and deletes expired keys. Thirty-two clients means thirty-two goroutines, plus that cleaner.

Redis, in the setup I benchmarked, runs the commands on one thread. That is the gap the 32-client number is about.

One connection is still handled one command after another. The extra cores only show up when many connections are busy together. And the goroutines do nothing useful if they all queue on a single lock. That is the whole reason for the shards.

**16 shards.** One shard is one Go map and one lock. The key is hashed, the hash picks the map. Keys on different maps can be touched at the same time. Keys on the same map take turns.

I timed parallel reads of 1,000 warm keys, with the keys already built, on this laptop:

| shards | time per read |
| --- | ---: |
| 1 | 92 ns |
| 2 | 56 ns |
| 4 | 33 ns |
| 8 | 20 ns |
| 16 | 13 ns |
| 32 | 10 ns |
| 64 | 10 ns |

Reads kept getting faster through 32 shards. 64 did not beat 32. The default is still 16, because that is the setting behind the Redis comparison further down. `-max-shards` changes it without a code edit. The constant is `defaultShardCount` in `internal/storage/store.go`. A bigger machine does not retune it.

The hash is FNV-1a, 32-bit. It is small, it does not allocate, and every command runs it, so that matters. `2166136261` and `16777619` are just the standard start value and multiplier for 32-bit FNV-1a. Same key, same shard, for as long as the process is up. If the hash comes out 0 I force it to 1. It is only there to spread keys. It is not hiding anything.

**Expiry on the read, plus a cleaner once a second.** `GET` looks at the deadline before it returns anything. If the time has passed, you get an empty reply, same as a missing key. You don't wait for a background job.

That still leaves keys nobody ever reads. They would sit there until restart. A worker wakes up once a second and deletes whatever is due. Once a second is enough, because the read is already correct. Doing it faster would burn CPU to free the same memory a little sooner.

Inside the process the deadline is Unix time in nanoseconds. Zero means no expiry.

**60 slots.** Each shard has a wheel with 60 buckets, one per second in a minute.

The deadline is rounded up to a whole second. That second mod 60 picks the bucket. A key expiring at second 125 goes in bucket 5, because `125 % 60 = 5`. The map entry remembers the bucket and the position, so an update can find the slot without scanning.

The worker, when it wakes, only looks at the current second's bucket on each shard. It does not walk the whole map. One second of a 100,000-key set is about 1,667 keys (`100000 / 60`). Deleting those took about 151 microseconds. If all 100,000 were due in the same second, it took about 12 milliseconds.

I used 60 and not 3600 (one slot per second of an hour) because the worker only needs "what might be due this second?" A longer wheel is a lot of empty slices for the same once-a-second wake-up. A key that expires next minute can land in the same bucket. If it is not actually due yet, the worker puts it back.

**One slot per key.** The first wheel appended a new slot on every `SET`, even when the key was already there. The map had one value. The wheel had a pile of old deadlines. Ten rewrites of 50,000 keys grew the process by 50.6 MiB. Reads were still correct, because a delete checked that the deadline matched. The RAM was the bug.

Now a live key owns one slot. `SET` it again and, if the new deadline is in the same bucket, the time in that slot is overwritten. If it moved to another bucket, the old slot is removed and one new one is added. `DEL`, or a `SET` with no TTL, removes the slot. Map and wheel update under the same lock, so they can't drift apart.

The same overwrite test now grows by about 9 MiB. That is spare slice space and heap Go has not given back to the OS. There is a test that rewrites 1,000 keys ten times and checks there is still one slot each.

**Go's map.** Each shard is a normal `map[string]Entry`. The entry is the value and the expiry time.

I tried a packed table on this same workload. It used more RAM than the map. The table grew in big empty steps, and each cell was fatter than a map entry. I threw it out.

That is also why a key is about 325 bytes of process RAM here and about 160 in Redis. Most of the gap is the map, not a second copy of the key string. Beating it means a layout that is actually tighter than Go's map on this workload, which the one I tried was not.

**`-max-memory`.** This is the sum of `len(key) + len(value)` across every key. `0` means no limit. All the shards share one counter. A `SET` that would pass the cap first drops the key with the soonest deadline. That key is already on the wheel, so this does not scan the whole map. Keys that never expire are not dropped. If the new value still does not fit, the write is refused and the previous value stays. A delete, an expiry, or a smaller value always adjusts the counter down.

## Commands

| You send | You get |
| --- | --- |
| `PING` | `PONG` |
| `SET key value` | `OK` |
| `SET key value EX 60` | `OK`, gone in 60 seconds |
| `SET key value PX 1500` | `OK`, gone in 1500 milliseconds |
| `GET key` | the value, or empty if it is missing or expired |
| `MGET k1 k2 k3` | one slot per key, same order, empty where it missed |
| `DEL key` | `1` if it was removed, `0` if it was not there |
| `EXPIRE key 60` | `1` if the key existed, `0` if it did not |
| `TTL key` | seconds left, rounded up. `-1` if it has no expiry. `-2` if it is missing or already due |

`DEL` is one key. `SET` only understands `EX` and `PX`. Anything else comes back as an error, either unknown command or wrong number of arguments.

## Reading the benchmark

One run, on the laptop above. Redis had snapshots off, the append-only file off, and one I/O thread. Half the timed commands are `GET`, half are `SET`.

I picked the workload like this:

- **50,000 keys** so RAM shows up, and so it still finishes on a laptop.
- **64-byte values** because that is a session id or a small JSON blob, not a document.
- **600 second expiry** so nothing expires during the 10 second measurement, but the wheel still has real deadlines in it.
- **1 client, then 32.** One client is a single connection, and it is noisy. 32 is more than the 12 CPUs, which is closer to a pool of app servers.
- **Batches of 16** because that is a normal amount for a Redis client to write before it reads. The server does not cap the batch there.
- **2 seconds of warmup**, not counted, so startup is not in the rate.
- **10 seconds measured**, long enough that the rate settles.
- **10 full rewrites** of the same keys, to see if memory grows when you overwrite.

commands/sec is counted at the client.

p50 is the typical wait. Half the commands were faster, half were slower.

p99 is the slow tail. 99 out of 100 were at least this fast.

For a batch of 16, the p99 is the wait for the whole batch, not for one command inside it.

RSS is what the OS says the process is using (`VmRSS` in `/proc/<pid>/status`).

bytes/key is `(RSS after load − RSS before any keys) / 50000`. That includes the map, the value, the wheel, and spare heap. It is not just the 64 bytes you stored.

| | Kairo | Redis |
| --- | ---: | ---: |
| commands/sec, 1 client | 13936 | 11731 |
| p50, 1 client | 63µs | 67µs |
| p99, 1 client | 216µs | 358µs |
| commands/sec, 32 clients | 130015 | 56512 |
| p50, 32 clients | 209µs | 514µs |
| p99, 32 clients | 1.035ms | 1.688ms |
| commands/sec, 32 clients, batches of 16 | 884296 | 327113 |
| p99 of a 16-command batch | 2.346ms | 3.82ms |
| RSS after load | 20.2 MiB | 20.8 MiB |
| bytes/key after load | 325 | 160 |
| RSS after 10 overwrites | 29.1 MiB | 20.9 MiB |
| growth from those overwrites | +8.9 MiB | +0.1 MiB |
| GET misses | 0 | 0 |

Again, the one-client row moved around between runs. The 32-client rows are the ones I trust.

Smaller timings, same machine, from `go test -bench`:

| | |
| --- | ---: |
| Read, 1 shard, many goroutines, precomputed keys | 92 ns |
| Read, 16 shards, many goroutines, precomputed keys | 13 ns |
| Read, 32 shards, many goroutines, precomputed keys | 10 ns |
| SET, no expiry | 76 ns |
| SET, 1 minute expiry | 165 ns |
| Refresh a key that stays in the same wheel bucket | 78 ns, nothing allocated |
| Delete ~1,667 keys due together | 151 µs |
| Delete one second of 100,000 keys spread around the wheel | 407 µs |
| Delete 100,000 keys all due in the same second | 12 ms |

The refresh line is the wheel fix. Updating a deadline in the same bucket allocates nothing. The old code allocated a new slot every time, which is where the 50.6 MiB came from.

## A local cache, and the other projects people use

A local cache is a map inside the app process. Ristretto, Otter, or the cache that already comes with the framework. Kairo is a different program, and the apps reach it over the network.

Say four servers are running the same app. With a map inside each one, server A can have the biryani page cached and server B still has to hit the database, because B cannot see A's memory. Change the price and delete the key on A, and B, C, and D can keep serving the old price until their own copies expire. A local read is just a function call, so it is faster when there is only one process and nothing else needs the data.

With Kairo, all four connect to port 8989 and see the same keys. The first request stores the page once. The other three read it. One `DEL` drops it for everybody. The cost is the network round trip.

Both forget their data when they restart. Restarting one app server only clears that server's local cache. Restarting Kairo clears the shared copy for every server at once.

Other things I would pick instead of Kairo, depending on the constraint:

**Redis or Valkey.** Same clients, plus saving to disk, replication, and the rest of the commands. Valkey is the open-source fork of Redis. This is the one to use when the data has to survive a restart, or when the app needs `INCR`, hashes, or lists. On this laptop Redis also used about half the RAM per key.

**Memcached.** A networked cache, older and simpler. Get, set, expiry, shared by many app servers. Use it when you want that shared cache and the apps do not already speak Redis.

**KeyDB, Dragonfly, and Garnet.** Multi-threaded, and they still speak a large part of Redis. These are the "many cores, but also a real Redis" projects. Kairo is the small version of that idea: many cores, only get, set, and a TTL, no disk.

**A cache inside the app.** Use this when there is one process and a network hop is wasted. Four app servers would be four separate copies.

Kairo is the narrow gap between a local map and Redis: one shared copy, Redis clients, no disk, and about twice the RAM per key.

## Still open

A key is still about twice the RAM of Redis. The packed table I tried used more than Go's map, so the map stayed. A key that never expires is not evicted. If every key is like that, or the new value alone is bigger than the cap, the write returns an error. There is no password, no disk, and no second machine.

## What I would do next

These fit the cache that is already here. They are not a second Redis.

**Reuse the value buffer on overwrite.** Every `SET` of an existing key allocates a new string and leaves the old one for the garbage collector. That is most of the ~9 MiB growth after ten rewrites of 50,000 keys. If the new value fits in the old buffer, copy it in. Redis already does this, which is why its RSS barely moves on the same test.

**Lock a shard once per `MGET`.** Each key currently takes and releases its shard lock on its own. A page that asks for several keys pays that once per key. Group the keys by shard and take each lock once.

**Run the Redis comparison again at 32 shards.** Parallel reads on this laptop were 13 ns at 16 shards and 10 ns at 32, and 64 did not beat 32. The 884k commands/sec number is still the 16-shard binary. I would rerun that same 32-client, pipeline-16 workload with `-max-shards 32` and change the default only if it still wins there.

**A small metrics page.** Counts for commands, hits, misses, evictions, and bytes used, on a separate port. Enough to see the process. Not a tracing project.

I am not planning persistence, replication, `INCR`, lists, or another hash table. Those are different products. Redis, Valkey, or Dragonfly already cover them.
