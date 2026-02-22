/**
 * ErrorBoundary — компонент-обёртка для перехвата ошибок рендеринга React.
 *
 * Если дочерний компонент выбрасывает ошибку при рендеринге,
 * ErrorBoundary перехватывает её и отображает экран ошибки
 * с возможностью повторной попытки (сброса состояния).
 *
 * Используется как обёртка вокруг корневого компонента приложения
 * для предотвращения полного краша UI при непредвиденных ошибках.
 */
import React from 'react';

/**
 * ErrorBoundaryProps — свойства компонента ErrorBoundary.
 * children — дочерние компоненты, которые оборачиваются обработчиком ошибок.
 */
interface ErrorBoundaryProps {
  children: React.ReactNode;
}

/**
 * ErrorBoundaryState — внутреннее состояние ErrorBoundary.
 * hasError — флаг наличия ошибки.
 * error — объект перехваченной ошибки (null, если ошибки нет).
 */
interface ErrorBoundaryState {
  hasError: boolean;
  error: Error | null;
}

/**
 * ErrorBoundary — классовый компонент React для перехвата ошибок рендеринга.
 *
 * При возникновении ошибки:
 * 1. getDerivedStateFromError — обновляет состояние (hasError=true).
 * 2. componentDidCatch — логирует ошибку и стек компонентов в консоль.
 * 3. render — отображает экран ошибки с кнопкой «Попробовать снова».
 */
class ErrorBoundary extends React.Component<ErrorBoundaryProps, ErrorBoundaryState> {
  constructor(props: ErrorBoundaryProps) {
    super(props);
    this.state = { hasError: false, error: null };
  }

  static getDerivedStateFromError(error: Error): ErrorBoundaryState {
    return { hasError: true, error };
  }

  componentDidCatch(error: Error, errorInfo: React.ErrorInfo): void {
    console.error('[ErrorBoundary]', error, errorInfo.componentStack);
  }

  handleReset = (): void => {
    this.setState({ hasError: false, error: null });
  };

  render(): React.ReactNode {
    if (this.state.hasError) {
      return (
        <div style={{
          display: 'flex',
          flexDirection: 'column',
          alignItems: 'center',
          justifyContent: 'center',
          height: '100vh',
          background: '#1e1e2e',
          color: '#cdd6f4',
          fontFamily: 'system-ui, sans-serif',
          padding: '2rem',
          textAlign: 'center',
        }}>
          <h1 style={{ fontSize: '1.5rem', marginBottom: '1rem', color: '#f38ba8' }}>
            Произошла ошибка
          </h1>
          <p style={{ maxWidth: '500px', marginBottom: '1.5rem', color: '#a6adc8' }}>
            {this.state.error?.message || 'Неизвестная ошибка в приложении'}
          </p>
          <button
            onClick={this.handleReset}
            style={{
              padding: '0.75rem 1.5rem',
              background: '#89b4fa',
              color: '#1e1e2e',
              border: 'none',
              borderRadius: '8px',
              cursor: 'pointer',
              fontSize: '1rem',
              fontWeight: 600,
            }}
          >
            Попробовать снова
          </button>
        </div>
      );
    }

    return this.props.children;
  }
}

export default ErrorBoundary;
