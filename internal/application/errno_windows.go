package application

import "syscall"

// addressInUseErrno — WSAEADDRINUSE (10048). На Windows syscall.EADDRINUSE
// не равен реальной ошибке занятого порта, поэтому проверяются оба значения.
const addressInUseErrno = syscall.Errno(10048)
