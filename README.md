# realm-profiler
A multi-threaded script to measure the response times of a gno.land RPC interface

This can be used for example to repeatedly test a malicious Gno script to see how it will affect the response time for benevolent RPC queries.

Consider for example the Gno contracts in `sample-innocent/` and `sample-attack/`. The "innocent" contract could represent benevolent queries, and the "attack" contract one that we're evaluating for risk to the chain.

```bash
cd sample-attack
go run ../profiler.go -mode addpkg+call -maxThreads 1 -maxQueriesPerSec 1
```
