import type { Direction } from "../config.js"

interface StatusBarProps {
  width: number
  message: string
  isTranslating: boolean
  direction: Direction
}

export function StatusBar({ width, message, isTranslating, direction }: StatusBarProps) {
  const dirLabel = direction === "zh2en" ? "ZH→EN" : "EN→ZH"
  const hints = ` ^T: 翻譯  ^L: 切換方向  ^Y: 複製  ^Q: 退出  [${dirLabel}]`
  const status = isTranslating ? " [翻譯中...]" : message ? ` ${message}` : ""

  return (
    <box width={width} height={1} flexDirection="row">
      <text content={hints} fg="#888888" />
      {status ? <text content={status} fg={message.includes("✓") ? "#44ff88" : "#ffaa44"} /> : null}
    </box>
  )
}
