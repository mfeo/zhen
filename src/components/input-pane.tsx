import type { TextareaRenderable } from "@opentui/core"
import { forwardRef, useEffect, useRef } from "react"
import type { Direction } from "../config.js"

interface InputPaneProps {
  width: number
  height: number
  direction: Direction
  onTextChange: (text: string) => void
}

export const InputPane = forwardRef<TextareaRenderable, InputPaneProps>(
  function InputPane({ width, height, direction, onTextChange }, forwardedRef) {
    const localRef = useRef<TextareaRenderable>(null)

    const setRef = (el: TextareaRenderable | null) => {
      ;(localRef as React.MutableRefObject<TextareaRenderable | null>).current = el
      if (typeof forwardedRef === "function") {
        forwardedRef(el)
      } else if (forwardedRef) {
        ;(forwardedRef as React.MutableRefObject<TextareaRenderable | null>).current = el
      }
    }

    useEffect(() => {
      const el = localRef.current
      if (!el) return
      el.onContentChange = () => {
        onTextChange(el.editBuffer.getText())
      }
      return () => {
        el.onContentChange = undefined
      }
    }, [onTextChange])

    const title = direction === "zh2en" ? " 中文輸入 " : " English Input "
    const placeholder =
      direction === "zh2en"
        ? "在這裡輸入中文... (Ctrl+T 翻譯)"
        : "Type English here... (Ctrl+T to translate)"

    return (
      <box
        width={width}
        height={height}
        border
        borderStyle="rounded"
        title={title}
        titleAlignment="center"
        flexDirection="column"
      >
        <textarea
          ref={setRef}
          flexGrow={1}
          focused
          placeholder={placeholder}
          wrapMode="word"
        />
      </box>
    )
  },
)
