// 像素组件桥接层：把 pixel-retroui 组件收敛为四面板消费的统一像素组件 API。
// 边界与取舍：
//   - 只做主题 props 收敛与 className 组合的薄封装，不复制 retroui 内部实现；
//     颜色与阴影不经组件 props 传值，而是由 tokens.css 像素组件段的变量映射
//     （--bg-button/--shadow-button 等）整体换肤，桥接层不出现裸色值。
//   - retroui 组件类自带 Minecraft 字体回退与演示用默认外距，统一在 ui.css
//     的 .pixel-* 公共呈现层覆盖；面板侧 class 只补布局几何与排印。
//   - 原生属性全量透传：disabled/aria-pressed/autoFocus/键盘事件等语义不因
//     桥接改变，上行事件仍由面板层回传，本层不接触桥协议。
import type { ButtonHTMLAttributes, InputHTMLAttributes } from "react";
import {
  Button as RetrouiButton,
  Card as RetrouiCard,
  DropdownMenu as RetrouiDropdownMenu,
  DropdownMenuContent as RetrouiDropdownMenuContent,
  DropdownMenuItem as RetrouiDropdownMenuItem,
  DropdownMenuTrigger as RetrouiDropdownMenuTrigger,
  type CardProps as RetrouiCardProps,
} from "pixel-retroui";

// joinClass 把公共呈现层 class 与调用方 class 组合为一个 className：
// retroui 组件的 className 形参是单值字符串，组合顺序不影响层叠
// （先后由 ui.css 的源序决定），这里只保证空 class 不产生多余空白。
function joinClass(base: string, className: string | undefined): string {
  return className === undefined ? base : `${base} ${className}`;
}

/** PixelButton 属性：原生按钮属性全量透传（禁用态、aria-pressed、键盘激活语义保持原生）。 */
export type PixelButtonProps = ButtonHTMLAttributes<HTMLButtonElement>;

// PixelButton：retroui Button 的像素化按钮。注意 retroui 的像素描边走
// border-image，border-color 悬停/焦点改色不可见——焦点几何标记由 ui.css
// 的 sage 外描环（outline）承担，选中面（aria-pressed）经表面变量切换
// --accent-wash（sage 水洗）。
export function PixelButton({ className, ...props }: PixelButtonProps) {
  return <RetrouiButton className={joinClass("pixel-button", className)} {...props} />;
}

/** PixelInput 属性：原生 input 属性全量透传；className 落在包装层，
 * 其余属性（value/onChange/aria-label/autoFocus 等）透传给内部原生 input，
 * label 包裹关联与可及性命名因此保持原生语义。 */
export type PixelInputProps = InputHTMLAttributes<HTMLInputElement>;

// `PixelInput` 用原生输入框保持表单语义，包装层统一奶油细描边与焦点环。
// retroui 的内联 border-image-source 会覆盖主题，因此此处不使用其图像边框。
export function PixelInput({ className, ...props }: PixelInputProps) {
  return <div className={joinClass("pixel-input", className)}><input {...props} /></div>;
}

/** PixelCard 属性：沿用 retroui Card 的形参面（面板容器几何不在组件内展开，
 * 布局几何由调用方 class 承担）。 */
export type PixelCardProps = RetrouiCardProps;

// PixelCard：retroui Card 的像素化容器。四面板的既有面板 class 已按令牌
 // 自带表面与投影，是否换用由各面板按布局风险自行取舍，桥接层不强制。
export function PixelCard({ className, ...props }: PixelCardProps) {
  return <RetrouiCard className={joinClass("pixel-card", className)} {...props} />;
}

/** PixelDropdown 的选项与受控形参：value/onChange 受控语义对齐原生 select 的最小面。 */
export interface PixelDropdownOption {
  value: string;
  label: string;
}

export interface PixelDropdownProps {
  options: readonly PixelDropdownOption[];
  value: string;
  onChange: (value: string) => void;
  /** 触发器的可及性命名：retroui 菜单件不参与表单 label 关联，需显式提供。 */
  ariaLabel?: string;
  className?: string;
}

// PixelDropdown：retroui DropdownMenu（点击弹出菜单件，非表单 select）的
// 受控薄封装。上游限制如实透传——菜单项是普通 div（无 option 角色、无键盘
// 导航），选中后也不自动收起，故本项目内它只适用于无强键盘契约的呈现面；
// 设置页窗口大小这类表单选择保持三按钮组（aria-pressed）语义，焦点顺序与
// 键盘语义与改造前逐项一致，不换用本组件。
export function PixelDropdown({ options, value, onChange, ariaLabel, className }: PixelDropdownProps) {
  const current = options.find((option) => option.value === value);
  return (
    <RetrouiDropdownMenu className={joinClass("pixel-dropdown", className)}>
      <RetrouiDropdownMenuTrigger aria-label={ariaLabel}>{current?.label ?? value}</RetrouiDropdownMenuTrigger>
      <RetrouiDropdownMenuContent>
        {/* 上游 DropdownMenuItem 不透传事件，选项以原生按钮承载点击与键盘激活。 */}
        {options.map((option) => (
          <RetrouiDropdownMenuItem key={option.value}>
            <button
              type="button"
              className="pixel-dropdown-option"
              onClick={() => {
                onChange(option.value);
              }}
            >
              {option.label}
            </button>
          </RetrouiDropdownMenuItem>
        ))}
      </RetrouiDropdownMenuContent>
    </RetrouiDropdownMenu>
  );
}
