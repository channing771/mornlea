import * as React from 'react';
import { cn } from '@/lib/utils';

// 表格样式纪律：行间只用单条 hairline 分隔（border-b，不上下夹线），
// 无外壳卡片、无投影；hover 底色瞬时切换（无过渡，符合动效纪律）。
function Table({ className, ...props }: React.HTMLAttributes<HTMLTableElement>) {
  return (
    <div className="relative w-full overflow-auto">
      <table className={cn('w-full caption-bottom text-sm', className)} {...props} />
    </div>
  );
}

// 表头行自身的 border-b 由 TableRow 基座提供（与此处重复声明同一属性，去掉表头这一份）。
function TableHeader({ className, ...props }: React.HTMLAttributes<HTMLTableSectionElement>) {
  return <thead className={cn(className)} {...props} />;
}

function TableBody({ className, ...props }: React.HTMLAttributes<HTMLTableSectionElement>) {
  return <tbody className={cn('[&_tr:last-child]:border-0', className)} {...props} />;
}

function TableRow({ className, ...props }: React.HTMLAttributes<HTMLTableRowElement>) {
  return <tr className={cn('border-b hover:bg-muted/30', className)} {...props} />;
}

// 表头：小号 sans-normal、muted 色（中文不做字距转换）。
function TableHead({ className, ...props }: React.ThHTMLAttributes<HTMLTableCellElement>) {
  return <th scope="col" className={cn('h-9 px-3 text-left align-middle text-xs font-normal text-muted-foreground', className)} {...props} />;
}

function TableCell({ className, ...props }: React.TdHTMLAttributes<HTMLTableCellElement>) {
  return <td className={cn('p-3 align-middle', className)} {...props} />;
}

export { Table, TableHeader, TableBody, TableRow, TableHead, TableCell };
