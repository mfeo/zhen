interface StatusBarProps {
  width: number
  message: string
  isTranslating: boolean
}

export function StatusBar({ width, message, isTranslating }: StatusBarProps) {
  const hints = " ^T: 翻譯  ^Y: 複製  ^Q: 退出"
  const status = isTranslating ? " [翻譯中...]" : message ? ` ${message}` : ""

  return (
    <box width={width} height={1} flexDirection="row">
      <text content={hints} fg="#888888" />
      {status ? <text content={status} fg={message.includes("✓") ? "#44ff88" : "#ffaa44"} /> : null}
    </box>
  )
}
