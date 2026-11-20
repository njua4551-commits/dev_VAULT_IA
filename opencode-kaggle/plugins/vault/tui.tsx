import { Plugin } from "@opencode/plugin/tui"
import { spawn } from "node:child_process"

export default Plugin.define({
  id: "opencode-n.portable-vault",
  setup(ctx) {
    let busy = false
    async function run(action: string) {
      const launcher = process.env.OPENCODE_VAULT_LAUNCHER
      if (busy || !launcher) return
      busy = true
      try {
        await ctx.renderer.suspend()
        try {
          const win = process.platform === "win32"
          const argv = action === "gpu" ? ["vault", "status"] : [action]
          const args = win ? ["-NoProfile", "-ExecutionPolicy", "Bypass", "-File", launcher, ...argv] : argv
          const status = await new Promise<number | null>((resolve, reject) => {
            const child = spawn(win ? "powershell.exe" : launcher, args, { stdio: "inherit" })
            child.once("error", reject)
            child.once("exit", resolve)
          })
          if (process.stdin.isTTY) {
            const { createInterface } = await import("node:readline/promises")
            const terminal = createInterface({ input: process.stdin, output: process.stdout })
            try { await terminal.question("\nEnter para volver al chat…") } finally { terminal.close() }
          }
          if (status !== 0) throw new Error("Vault: operación incompleta")
        } finally {
          await ctx.renderer.resume()
        }
        ctx.ui.toast.show({ title: "Vault", message: action === "up" ? "Selecciona Kaggle en /models." : "De vuelta al chat.", variant: "info" })
      } catch {
        ctx.ui.toast.show({ title: "Vault", message: "No se completó. Ejecuta opencode_n vault en la terminal para ver el detalle.", variant: "warning" })
      } finally {
        busy = false
      }
    }
    return ctx.ui.slot({ append: "app", render() {
      ctx.keymap.layer(() => ({ mode: "global", commands: [
        ...[["vault", "Configurar y gestionar Kaggle"], ["up", "Encender y conectar Kaggle"], ["down", "Apagar Kaggle"], ["gpu", "Consultar cuota de GPU"]].map(([name, title]) => ({
          id: `opencode-n.portable-vault.${name}`, title, group: "Vault", palette: true, slash: { name }, run: () => run(name),
        })),
      ] }))
      return null
    } })
  },
})
