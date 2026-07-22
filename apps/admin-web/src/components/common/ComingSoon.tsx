import { Construction } from 'lucide-react'

interface ComingSoonProps {
  title: string
  reason?: string
}

/**
 * 占位组件：用于后端 API 尚未迁移到 YunDu Go 微服务的页面。
 * 后端 API 实现后，移除页面中的 <ComingSoon /> return 即可恢复原逻辑。
 */
export function ComingSoon({ title, reason }: ComingSoonProps) {
  return (
    <div className="flex flex-col items-center justify-center py-20 px-4">
      <div className="w-16 h-16 rounded-2xl bg-amber-500/10 flex items-center justify-center mb-4 border border-amber-500/20">
        <Construction className="w-8 h-8 text-amber-400" />
      </div>
      <h2 className="text-xl font-bold text-zinc-200 mb-2">{title}</h2>
      <p className="text-sm text-zinc-500 text-center max-w-md leading-relaxed">
        {reason || '该模块正在从 XBoard 迁移到 YunDu Go 微服务，后端 API 完成后自动恢复。'}
      </p>
    </div>
  )
}
