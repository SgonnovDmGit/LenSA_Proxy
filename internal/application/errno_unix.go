//go:build unix

package application

import "syscall"

// addressInUseErrno — EADDRINUSE. На unix-платформах совпадает с
// syscall.EADDRINUSE; проверка обоих значений в lifecycleErrorMessage
// унифицирована с Windows.
const addressInUseErrno = syscall.EADDRINUSE
