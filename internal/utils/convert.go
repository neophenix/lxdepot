package utils

import "fmt"

func MakeBytesMoreHuman(bytes uint64) string {
    switch {
        case bytes >= 1000000000:
            return fmt.Sprintf("%v GB", bytes / 1073741824)
        case bytes >= 1000000:
            return fmt.Sprintf("%v MB", bytes / 1048576)
        case bytes >= 1000:
            return fmt.Sprintf("%v KB", bytes / 1024)
    }

    return fmt.Sprintf("%v B", bytes)
}

func MakeIntBytesMoreHuman(bytes int64) string {
    return MakeBytesMoreHuman(uint64(bytes))
}
