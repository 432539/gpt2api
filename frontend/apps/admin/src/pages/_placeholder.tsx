import { Construction } from 'lucide-react';

interface Props {
  title: string;
  desc: string;
  hint?: string;
}

export function PlaceholderPage({ title, desc, hint }: Props) {
  return (
    <div className="page page-wide space-y-4">
      <header className="page-header">
        <div>
          <h1 className="page-title">{title}</h1>
          <p className="page-subtitle">{desc}</p>
        </div>
      </header>

      <div className="card">
        <div className="empty-state">
          <div className="empty-state-icon">
            <Construction size={28} />
          </div>
          <p className="empty-state-title">该模块开发中</p>
          <p className="empty-state-desc">
            {hint ?? '正在对接对应 admin API；上线后会替换此处占位界面，请稍候。'}
          </p>
        </div>
      </div>
    </div>
  );
}
