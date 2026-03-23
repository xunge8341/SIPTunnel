# Task 9 / Task 10 实验执行手册

- Task 9：评估 Windows + Go1.26 keep-alive workaround 对吞吐的影响
- Task 10：形成 3 / 5 / 10 并发下的容量基线压测矩阵

## 入口

- `scripts/loadtest/run_keepalive_ab.sh`
- `scripts/loadtest/run_capacity_matrix.sh`
- `go run ./cmd/loadtest -analyze-experiment <manifest> -experiment-output <report>`
