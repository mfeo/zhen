import clipboard from "clipboardy"

export async function copyToClipboard(text: string): Promise<void> {
  // Try OSC 52 first (works over SSH, tmux, etc.)
  try {
    const encoded = Buffer.from(text).toString("base64")
    process.stdout.write(`\x1b]52;c;${encoded}\x07`)
  } catch {
    // ignore
  }

  // Also write via native clipboard for local sessions
  await clipboard.write(text)
}
