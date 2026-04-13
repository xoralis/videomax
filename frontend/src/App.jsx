import { useState } from 'react';
import { BrowserRouter, Link, Navigate, Route, Routes, useNavigate } from 'react-router-dom';
import CreateForm from './components/CreateForm';
import ProgressView from './components/ProgressView';
import LoginForm from './components/auth/LoginForm';
import RegisterForm from './components/auth/RegisterForm';
import HistoryPage from './components/history/HistoryPage';
import { getUser, isAuthenticated, logout } from './services/authService';

// 需要登录才能访问的路由包装
function PrivateRoute({ children }) {
  return isAuthenticated() ? children : <Navigate to="/login" replace />;
}

// 主创建页（保持原有逻辑）
function HomePage() {
  const [currentTaskId, setCurrentTaskId] = useState(null);
  return (
    <main className="w-full max-w-7xl mx-auto flex flex-col items-center justify-center min-h-[80vh] pt-4 pb-16 px-4 sm:px-6 lg:px-8">
      {!currentTaskId ? (
        <CreateForm onTaskCreated={setCurrentTaskId} />
      ) : (
        <ProgressView taskId={currentTaskId} onBack={() => setCurrentTaskId(null)} />
      )}
    </main>
  );
}

// 顶部导航栏
function Navbar() {
  const navigate = useNavigate();
  const user = getUser();

  const handleLogout = () => {
    logout();
    navigate('/login');
  };

  return (
    <header className="fixed top-0 left-0 right-0 z-50 bg-slate-900/80 backdrop-blur border-b border-slate-800">
      <div className="max-w-7xl mx-auto flex items-center justify-between px-4 sm:px-8 h-12">
        <Link to="/" className="text-white font-bold text-lg tracking-tight">
          video<span className="text-cyan-400">Max</span>
        </Link>
        <div className="flex items-center gap-4">
          {isAuthenticated() ? (
            <>
              <Link to="/history" className="text-slate-400 hover:text-white text-sm transition-colors">
                历史记录
              </Link>
              <span className="text-slate-500 text-sm hidden sm:block">{user?.username}</span>
              <button
                onClick={handleLogout}
                className="text-slate-400 hover:text-red-400 text-sm transition-colors"
              >
                退出
              </button>
            </>
          ) : (
            <Link to="/login" className="text-cyan-400 hover:text-cyan-300 text-sm transition-colors">
              登录
            </Link>
          )}
        </div>
      </div>
    </header>
  );
}

function AppLayout() {
  return (
    <div className="min-h-screen bg-slate-900 text-white font-sans selection:bg-cyan-500/30 overflow-x-hidden">
      {/* 背景网格 */}
      <div className="fixed inset-0 z-0 pointer-events-none">
        <div className="absolute inset-0 bg-[url('data:image/svg+xml;base64,PHN2ZyB3aWR0aD0iMjQiIGhlaWdodD0iMjQiIHhtbG5zPSJodHRwOi8vd3d3LnczLm9yZy8yMDAwL3N2ZyI+CjxwYXRoIGQ9Ik0gMjQgMCBMMCAwIDAgMjQiIGZpbGw9Im5vbmUiIHN0cm9rZT0icmdiYSgyNTUsIDI1NSwgMjU1LCAwLjAyKSIgc3Ryb2tlLXdpZHRoPSIxIiAvPgo8L3N2Zz4=')] opacity-50" />
      </div>

      <Navbar />

      <div className="relative z-10 pt-12">
        <Routes>
          <Route path="/login" element={<LoginForm />} />
          <Route path="/register" element={<RegisterForm />} />
          <Route path="/" element={<PrivateRoute><HomePage /></PrivateRoute>} />
          <Route path="/history" element={<PrivateRoute><HistoryPage /></PrivateRoute>} />
          <Route path="*" element={<Navigate to="/" replace />} />
        </Routes>
      </div>

      <footer className="fixed bottom-0 w-full p-4 text-center text-slate-500 text-xs z-10">
        videoMax © {new Date().getFullYear()} — Multi-Agent AI Video Engine
      </footer>
    </div>
  );
}

function App() {
  return (
    <BrowserRouter>
      <AppLayout />
    </BrowserRouter>
  );
}

export default App;

