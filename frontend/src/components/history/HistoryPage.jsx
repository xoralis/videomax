import { useEffect, useState } from 'react';
import { getStats, getTasks } from '../../services/historyService';

const STATUS_LABEL = {
  pending: { text: '等待中', cls: 'bg-slate-600 text-slate-200' },
  phase_story: { text: '故事策划', cls: 'bg-blue-600 text-blue-100' },
  phase_char: { text: '角色设定', cls: 'bg-blue-600 text-blue-100' },
  phase_board: { text: '分镜规划', cls: 'bg-blue-600 text-blue-100' },
  phase_visual: { text: '画面生成', cls: 'bg-violet-600 text-violet-100' },
  phase_review: { text: '质检审核', cls: 'bg-yellow-600 text-yellow-100' },
  generating: { text: '视频生成中', cls: 'bg-amber-500 text-white' },
  success: { text: '已完成', cls: 'bg-emerald-600 text-emerald-100' },
  failed: { text: '失败', cls: 'bg-red-600 text-red-100' },
};

function StatusBadge({ status }) {
  const s = STATUS_LABEL[status] || { text: status, cls: 'bg-slate-600 text-slate-200' };
  return (
    <span className={`inline-block text-xs font-medium px-2 py-0.5 rounded-full ${s.cls}`}>
      {s.text}
    </span>
  );
}

function StatCard({ label, value, sub }) {
  return (
    <div className="bg-slate-800 border border-slate-700 rounded-xl p-4 flex flex-col gap-1">
      <span className="text-slate-400 text-xs">{label}</span>
      <span className="text-white text-2xl font-bold">{value}</span>
      {sub && <span className="text-slate-500 text-xs">{sub}</span>}
    </div>
  );
}

export default function HistoryPage() {
  const [stats, setStats] = useState(null);
  const [tasks, setTasks] = useState([]);
  const [total, setTotal] = useState(0);
  const [page, setPage] = useState(1);
  const pageSize = 10;
  const [loading, setLoading] = useState(true);
  const [error, setError] = useState('');

  useEffect(() => {
    loadAll();
  }, []);

  useEffect(() => {
    loadTasks();
  }, [page]);

  async function loadAll() {
    setLoading(true);
    try {
      const [statsRes, tasksRes] = await Promise.all([getStats(), getTasks(1, pageSize)]);
      setStats(statsRes.stats);
      setTasks(tasksRes.tasks || []);
      setTotal(tasksRes.total || 0);
    } catch (err) {
      setError(err.message);
    } finally {
      setLoading(false);
    }
  }

  async function loadTasks() {
    try {
      const res = await getTasks(page, pageSize);
      setTasks(res.tasks || []);
      setTotal(res.total || 0);
    } catch (err) {
      setError(err.message);
    }
  }

  const totalPages = Math.ceil(total / pageSize);
  const successRate = stats && stats.total > 0
    ? Math.round((stats.success_count / stats.total) * 100)
    : 0;

  return (
    <div className="min-h-screen bg-slate-900 text-white px-4 sm:px-8 py-10 max-w-5xl mx-auto">
      <h1 className="text-2xl font-bold mb-6">我的历史记录</h1>

      {/* 统计卡片 */}
      {stats && (
        <div className="grid grid-cols-2 sm:grid-cols-4 gap-4 mb-8">
          <StatCard label="总任务数" value={stats.total} />
          <StatCard label="成功" value={stats.success_count} sub={`成功率 ${successRate}%`} />
          <StatCard label="失败" value={stats.failed_count} />
          <StatCard label="进行中" value={stats.in_progress_count} />
        </div>
      )}

      {/* 模型分布 */}
      {stats && Object.keys(stats.model_distribution || {}).length > 0 && (
        <div className="bg-slate-800 border border-slate-700 rounded-xl p-4 mb-8">
          <h2 className="text-slate-400 text-sm mb-3">模型使用分布</h2>
          <div className="flex flex-wrap gap-3">
            {Object.entries(stats.model_distribution).map(([model, count]) => (
              <div key={model} className="flex items-center gap-2">
                <span className="text-xs bg-slate-700 rounded-full px-3 py-1 text-cyan-300">{model}</span>
                <span className="text-slate-400 text-xs">{count} 次</span>
              </div>
            ))}
          </div>
        </div>
      )}

      {/* 任务列表 */}
      {error && <p className="text-red-400 mb-4">{error}</p>}
      {loading ? (
        <p className="text-slate-400">加载中...</p>
      ) : tasks.length === 0 ? (
        <div className="text-center text-slate-500 py-16">暂无历史记录，去创建第一个视频吧！</div>
      ) : (
        <>
          <div className="space-y-3">
            {tasks.map((task) => (
              <div
                key={task.id}
                className="bg-slate-800 border border-slate-700 rounded-xl p-4 flex flex-col sm:flex-row sm:items-center gap-3"
              >
                <div className="flex-1 min-w-0">
                  <p className="text-white text-sm font-medium truncate">{task.original_idea}</p>
                  <p className="text-slate-500 text-xs mt-1">
                    {task.model && <span className="mr-3">{task.model}</span>}
                    {new Date(task.created_at).toLocaleString('zh-CN')}
                  </p>
                </div>
                <div className="flex items-center gap-3 shrink-0">
                  <StatusBadge status={task.status} />
                  {task.video_url && (
                    <a
                      href={task.video_url}
                      target="_blank"
                      rel="noopener noreferrer"
                      className="text-xs bg-cyan-600 hover:bg-cyan-500 text-white px-3 py-1 rounded-full transition-colors"
                    >
                      观看视频
                    </a>
                  )}
                </div>
              </div>
            ))}
          </div>

          {/* 分页 */}
          {totalPages > 1 && (
            <div className="flex justify-center gap-2 mt-6">
              <button
                disabled={page === 1}
                onClick={() => setPage(page - 1)}
                className="px-3 py-1.5 rounded-lg bg-slate-700 text-slate-300 disabled:opacity-40 hover:bg-slate-600 text-sm"
              >
                上一页
              </button>
              <span className="px-3 py-1.5 text-slate-400 text-sm">{page} / {totalPages}</span>
              <button
                disabled={page === totalPages}
                onClick={() => setPage(page + 1)}
                className="px-3 py-1.5 rounded-lg bg-slate-700 text-slate-300 disabled:opacity-40 hover:bg-slate-600 text-sm"
              >
                下一页
              </button>
            </div>
          )}
        </>
      )}
    </div>
  );
}
