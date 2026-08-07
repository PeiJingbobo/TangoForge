# -*- coding: utf-8 -*-
import io

css = """

/* ─────────────────────────────────────────────────────────────
 * 过渡动画（TF 抽屉/弹窗；无 tw-animate 依赖，手写 keyframes）：
 * Radix Dialog 在 data-state=open/closed 期间保持挂载，等待动画结束再卸载。
 * ───────────────────────────────────────────────────────────── */
@keyframes tf-fade-in {
  from { opacity: 0; }
  to { opacity: 1; }
}
@keyframes tf-fade-out {
  from { opacity: 1; }
  to { opacity: 0; }
}
@keyframes tf-sheet-in-right {
  from { transform: translateX(100%); }
  to { transform: translateX(0); }
}
@keyframes tf-sheet-out-right {
  from { transform: translateX(0); }
  to { transform: translateX(100%); }
}
@keyframes tf-dialog-in {
  from { opacity: 0; transform: translate(-50%, -48%) scale(0.96); }
  to { opacity: 1; transform: translate(-50%, -50%) scale(1); }
}
@keyframes tf-dialog-out {
  from { opacity: 1; transform: translate(-50%, -50%) scale(1); }
  to { opacity: 0; transform: translate(-50%, -50%) scale(0.96); }
}

[data-slot='sheet-overlay'][data-state='open'],
[data-slot='dialog-overlay'][data-state='open'] {
  animation: tf-fade-in 0.25s ease-out;
}
[data-slot='sheet-overlay'][data-state='closed'],
[data-slot='dialog-overlay'][data-state='closed'] {
  animation: tf-fade-out 0.2s ease-in;
}
[data-slot='sheet-content'][data-state='open'] {
  animation: tf-sheet-in-right 0.3s cubic-bezier(0.16, 1, 0.3, 1);
}
[data-slot='sheet-content'][data-state='closed'] {
  animation: tf-sheet-out-right 0.25s ease-in;
}
[data-slot='dialog-content'][data-state='open'] {
  animation: tf-dialog-in 0.22s ease-out;
}
[data-slot='dialog-content'][data-state='closed'] {
  animation: tf-dialog-out 0.18s ease-in;
}
"""

p = 'app/src/styles/globals.css'
s = io.open(p, encoding='utf-8').read()
if '[data-slot=\'sheet-content\']' not in s:
    s = s + css
    io.open(p, 'w', encoding='utf-8', newline='\n').write(s)
    print('animations appended')
else:
    print('already present')
