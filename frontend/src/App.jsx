import React, { useState } from 'react';
import CreateForm from './components/CreateForm';
import ProgressView from './components/ProgressView';

function App() {
  const [currentTaskId, setCurrentTaskId] = useState(null);

  const handleTaskCreated = (taskId) => {
    setCurrentTaskId(taskId);
  };

  const handleBackToCreate = () => {
    setCurrentTaskId(null);
  };

  return (
    <main className="min-h-screen bg-slate-900 text-white font-sans selection:bg-cyan-500/30 overflow-x-hidden pt-12 pb-24 px-4 sm:px-6 lg:px-8 relative">
      
      {/* Global Background Elements */}
      <div className="fixed inset-0 z-0 pointer-events-none">
        {/* Subtle grid pattern */}
        <div className="absolute inset-0 bg-[url('data:image/svg+xml;base64,PHN2ZyB3aWR0aD0iMjQiIGhlaWdodD0iMjQiIHhtbG5zPSJodHRwOi8vd3d3LnczLm9yZy8yMDAwL3N2ZyI+CjxwYXRoIGQ9Ik0gMjQgMCBMMCAwIDAgMjQiIGZpbGw9Im5vbmUiIHN0cm9rZT0icmdiYSgyNTUsIDI1NSwgMjU1LCAwLjAyKSIgc3Ryb2tlLXdpZHRoPSIxIiAvPgo8L3N2Zz4=')] opacity-50" />
      </div>

      <div className="relative z-10 w-full max-w-7xl mx-auto flex flex-col items-center justify-center min-h-[80vh]">
        {!currentTaskId ? (
          <CreateForm onTaskCreated={handleTaskCreated} />
        ) : (
          <ProgressView taskId={currentTaskId} onBack={handleBackToCreate} />
        )}
      </div>

      <footer className="fixed bottom-0 w-full p-4 text-center text-slate-500 text-xs z-10">
        videoMax © {new Date().getFullYear()} — Multi-Agent AI Video Engine
      </footer>
    </main>
  );
}

export default App;
