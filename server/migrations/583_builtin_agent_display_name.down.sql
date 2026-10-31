-- Reverses the batch-A display-name rename for rows this migration changed.
-- A description the product seeded is restored verbatim; a name is restored to
-- Mika only while it is still Siman, so an owner rename made after the up
-- migration is not silently undone beyond the product's own copy.
--
-- The mapping is the exact inverse of the up file, per language: Korean needs
-- Siman이 -> Mika가, not a name-only replacement, because the particle changes
-- with the name's final consonant.
--
-- Idempotent: after the first run the WHERE matches nothing.
UPDATE agent
SET name = CASE WHEN name = 'Siman' THEN 'Mika' ELSE name END,
    description = CASE description
        WHEN 'Your workspace Chief of Staff. Siman turns goals into issues, coordinates agents, and helps build reusable workflows.'
            THEN 'Your workspace Chief of Staff. Mika turns goals into issues, coordinates agents, and helps build reusable workflows.'
        WHEN '你的工作区 Chief of Staff。Siman 会把目标转化为任务、协调智能体，并帮你建立可复用的工作流。'
            THEN '你的工作区 Chief of Staff。Mika 会把目标转化为任务、协调智能体，并帮你建立可复用的工作流。'
        WHEN '워크스페이스의 Chief of Staff입니다. Siman이 목표를 태스크로 구체화하고 에이전트를 조율하며 재사용 가능한 워크플로 구성을 돕습니다.'
            THEN '워크스페이스의 Chief of Staff입니다. Mika가 목표를 태스크로 구체화하고 에이전트를 조율하며 재사용 가능한 워크플로 구성을 돕습니다.'
        WHEN 'ワークスペースの Chief of Staff。Siman は目標をタスクに落とし込み、エージェントを調整し、再利用できるワークフローづくりを支援します。'
            THEN 'ワークスペースの Chief of Staff。Mika は目標をタスクに落とし込み、エージェントを調整し、再利用できるワークフローづくりを支援します。'
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
