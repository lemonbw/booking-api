INSERT INTO rooms (id, name)
VALUES (1, 'Meeting Room Aurora'),
       (2, 'Meeting Room Borealis'),
       (3, 'Focus Pod 3')
ON CONFLICT (id) DO NOTHING;
