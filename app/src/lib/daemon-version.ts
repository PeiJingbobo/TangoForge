/**
 * 守护进程版本比对（TF-053，Electron 主进程 daemon.ts 复用）：
 * 判定「运行中的 daemon 是否与 APP 版本匹配」。
 * 抽为纯函数以便单测（主进程 fetch/electron 依赖不在 vitest 范围）。
 */

/** 版本是否需要重启：running 缺失/为空 → 无法判定（false，不重启）；不等 → true */
export function shouldRestartDaemon(required: string, running: string | null): boolean {
  if (!required || !running) return false
  return required !== running
}

/** 探测到版本不匹配后的可选动作：返回待确认的重启目标二进制路径（非空即可发起重启） */
export function restartTarget(bin: string | null): string {
  return bin ?? ''
}
