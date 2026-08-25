import { clsx, type ClassValue } from 'clsx';
import { twMerge } from 'tailwind-merge';

// cn 合并 className：clsx 处理条件类名，tailwind-merge 去重冲突的 Tailwind 工具类。
export function cn(...inputs: ClassValue[]) {
  return twMerge(clsx(inputs));
}
