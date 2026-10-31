-- Reverses the batch-A display-name rename for rows this migration changed.
-- A description the product seeded is restored verbatim; a name is restored to
-- Mika only while it is still Siman, so an owner rename made after the up
-- migration is not silently undone beyond the product's own copy.
UPDATE agent
SET name = CASE WHEN name = 'Siman' THEN 'Mika' ELSE name END,
    description = CASE
        WHEN description IN (
            'Your workspace Chief of Staff. Siman turns goals into issues, coordinates agents, and helps build reusable workflows.',
            '你的工作区 Chief of Staff。Siman 会把目标转化为任务、协调智能体，并帮你建立可复用的工作流。',
            '워크스페이스의 Chief of Staff입니다. Siman이 목표를 태스크로 구체화하고 에이전트를 조율하며 재사용 가능한 워크플로 구성을 돕습니다.',
            'ワークスペースの Chief of Staff。Siman は目標をタスクに落とし込み、エージェントを調整し、再利用できるワークフローづくりを支援します。'
        ) THEN REPLACE(description, 'Siman', 'Mika')
        ELSE description
    END,
    updated_at = now()
WHERE system_key = 'mika'
  AND (
      name = 'Siman'
      OR description IN (
          'Your workspace Chief of Staff. Siman turns goals into issues, coordinates agents, and helps build reusable workflows.',
          '你的工作区 Chief of Staff。Siman 会把目标转化为任务、协调智能体，并帮你建立可复用的工作流。',
          '워크스페이스의 Chief of Staff입니다. Siman이 목표를 태스크로 구체화하고 에이전트를 조율하며 재사용 가능한 워크플로 구성을 돕습니다.',
          'ワークスペースの Chief of Staff。Siman は目標をタスクに落とし込み、エージェントを調整し、再利用できるワークフローづくりを支援します。'
      )
  );
