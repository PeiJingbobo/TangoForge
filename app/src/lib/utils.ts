import { clsx, type ClassValue } from 'clsx'
import { twMerge } from 'tailwind-merge'

/** shadcn-ui 推荐的 cn() 工具（docs/TECHNICAL.md §4.1 lib/）。 */
export function cn(...inputs: ClassValue[]): string {
  return twMerge(clsx(inputs))
}
