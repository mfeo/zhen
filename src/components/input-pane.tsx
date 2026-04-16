import type { TextareaRenderable } from "@opentui/core"
import { useEffect, useRef } from "react"

interface InputPaneProps {
  width: number
  height: number
  onTextChange: (text: string) => void
}

export function InputPane({ width, height, onTextChange }: InputPaneProps) {
  const ref = useRef<TextareaRenderable>(null)

  useEffect(() => {
    const el = ref.current
    if (!el) return
    el.onContentChange = () => {
      onTextChange(el.editBuffer.getText())
    }
    return () => {
      el.onContentChange = undefined
    }
  }, [onTextChange])

  return (
    <box
      width={width}
      height={height}
      border
      borderStyle="rounded"
      title=" 中文輸入 "
      titleAlignment="center"
      flexDirection="column"
    >
      <textarea
        ref={ref}
        flexGrow={1}
        focused
        placeholder="在這裡輸入中文... (Ctrl+T 翻譯)"
        wrapMode="word"
      />
    </box>
  )
}
