import { Navigate, Route, Routes } from "react-router-dom";

import { useAuthState } from "@/lib/auth";
import AppLayout from "@/routes/AppLayout";
import AuthCallback from "@/routes/AuthCallback";
import Usage from "@/routes/Usage";
import Login from "@/routes/Login";
import NewRun from "@/routes/NewRun";
import RunDetail from "@/routes/RunDetail";
import Runs from "@/routes/Runs";
import Settings from "@/routes/Settings";
import ApiKeys from "@/routes/ApiKeys";

export default function App() {
  const auth = useAuthState();

  if (auth.status === "loading") {
    return (
      <div className="flex h-screen items-center justify-center text-muted-foreground">
        Loading...
      </div>
    );
  }

  return (
    <Routes>
      <Route path="/auth/callback" element={<AuthCallback />} />
      {auth.status === "signed_out" ? (
        <>
          <Route path="/login" element={<Login />} />
          <Route path="*" element={<Navigate to="/login" replace />} />
        </>
      ) : (
        <Route element={<AppLayout profile={auth.profile} />}>
          <Route index element={<Navigate to="/runs" replace />} />
          <Route path="/runs" element={<Runs />} />
          <Route path="/runs/new" element={<NewRun />} />
          <Route path="/runs/:runId" element={<RunDetail />} />
          <Route path="/usage" element={<Usage />} />
          <Route path="/billing" element={<Navigate to="/usage" replace />} />
          <Route path="/api-keys" element={<ApiKeys />} />
          <Route path="/tokens" element={<Navigate to="/api-keys" replace />} />
          <Route path="/settings" element={<Settings />} />
          <Route path="*" element={<Navigate to="/runs" replace />} />
        </Route>
      )}
    </Routes>
  );
}
