package server

// runtime_support.go 只保留运行态支撑域的边界说明；
// 具体实现已经按职责拆分到 access_log_store、loadtest、安全事件与默认设置文件中，
// 避免再次回到“一个文件混放多类运行态胶水”的状态。
