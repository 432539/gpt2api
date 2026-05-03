import { useCallback } from 'react';

import { useAuthStore } from '../stores/auth';
import { useLoginGateStore } from '../stores/loginGate';

/**
 * 调用 ensure(action[, hint]) 来执行需要登录态的动作：
 *   - 已登录：立刻同步执行
 *   - 未登录：打开登录浮层，登录成功后自动执行
 * 返回值：是否已经登录（false 表示已弹浮层等待用户登录）
 */
export function useEnsureLoggedIn() {
  const tokenSel = useAuthStore((s) => s.token);
  const openGate = useLoginGateStore((s) => s.openGate);

  return useCallback(
    (action: () => void, hint?: string): boolean => {
      if (tokenSel) {
        action();
        return true;
      }
      openGate({ hint: hint ?? '登录后即可生成', onLoggedIn: action });
      return false;
    },
    [tokenSel, openGate],
  );
}
