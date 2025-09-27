# dmesg Parser Example

이 예제는 freader의 dmesg 파서를 사용하여 Linux 커널 메시지를 파싱하는 방법을 보여줍니다.

## 기능

- **타임스탬프 파싱**: 부팅 후 경과 시간 추출
- **우선순위/Facility 추출**: syslog 형식 지원
- **서브시스템 인식**: usb, net, kernel, docker 등 자동 인식
- **절대 시간 계산**: 부팅 시간 설정시 실제 시간 계산
- **실시간 모니터링**: 파일 변경사항 실시간 감지

## 실행 방법

```bash
# freader 프로젝트 루트에서
go run ./examples/dmesg_parser
```

## 직접 파서 사용 예제

```go
package main

import (
    "fmt"
    "time"
    "github.com/loykin/freader/pkg/parser/dmesg"
)

func main() {
    parser := dmesg.NewParser()

    // 부팅 시간 설정 (선택사항)
    bootTime := time.Now().Add(-2 * time.Hour)
    parser.SetBootTime(bootTime)

    // dmesg 라인 파싱
    line := "[  100.500000] usb 1-1: new high-speed USB device number 2 using ehci-pci"

    record, err := parser.Parse(line)
    if err != nil {
        panic(err)
    }

    fmt.Printf("Timestamp: %.6f seconds\n", record.Timestamp)
    fmt.Printf("Subsystem: %s\n", record.Subsystem)
    fmt.Printf("Message: %s\n", record.Message)

    if record.AbsoluteTime != nil {
        fmt.Printf("Absolute Time: %s\n", record.AbsoluteTime.Format(time.RFC3339))
    }

    // JSON 출력
    jsonData, _ := parser.ParseJSON(line)
    fmt.Printf("JSON: %s\n", string(jsonData))
}
```

## 지원하는 dmesg 형식

1. **기본 형식**: `[timestamp] message`
2. **우선순위 포함**: `<priority>[timestamp] message`
3. **서브시스템 포함**: `[timestamp] subsystem: details`

## 샘플 로그

```
[    0.000000] Linux version 5.15.0-56-generic
[   10.123456] pci 0000:00:1f.3: [8086:a348] type 00 class 0x040300
<6>[   20.000000] systemd[1]: Started Load Kernel Modules.
[  100.500000] usb 1-1: new high-speed USB device number 2 using ehci-pci
[  200.000000] docker0: port 1(veth123abc) entered blocking state
```

## 파서 출력 예제

```json
{
  "raw": "[  100.500000] usb 1-1: new high-speed USB device number 2 using ehci-pci",
  "timestamp": 100.5,
  "subsystem": "usb",
  "message": "usb 1-1: new high-speed USB device number 2 using ehci-pci",
  "absolute_time": "2023-12-01T08:01:40.500Z"
}
```

## 활용 사례

- **시스템 모니터링**: 커널 이벤트 실시간 감시
- **하드웨어 진단**: USB, 네트워크, 디스크 오류 감지
- **성능 분석**: 시스템 부팅 시간 및 이벤트 순서 분석
- **로그 집계**: 구조화된 형태로 커널 로그 수집